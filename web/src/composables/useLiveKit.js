/**
 * useLiveKit — Vue composable wrapping LiveKit room connection and track management.
 * 支持 video / voice 两种房间模式 (视频房 ≤5 人, 语音房 ≤10 人).
 */
import { ref, reactive, shallowRef } from 'vue'
import { Room, RoomEvent, Track } from 'livekit-client'

// ── reactive state ──
const room = shallowRef(null)            // LiveKit Room instance
const isConnected = ref(false)
const isConnecting = ref(false)
const connectionError = ref('')

const micOn = ref(true)
const camOn = ref(true)

// Track storage: Map<identity, { track, name }>
const trackEntries = reactive(new Map())

// 参与者列表（语音模式 UI 用）: Map<identity, { name, speaking }>
const participantEntries = reactive(new Map())

// 远端音频元素: identity → HTMLAudioElement (音频不 attach 就没有声音; 断线时清理)
const audioElements = new Map()

// 远端音频播放状态诊断 (页面上直接显示, 排查自动播放策略问题)
const audioStatus = ref('')

// 自动播放解锁监听器 (每次 connect 注册, disconnect 时清理)
let unlockCleanup = []

// ── 实时音量显示 (device-test.html 同款算法: AnalyserNode + RMS → 0-100) ──
const localIdentity = ref('') // 本地参与者 identity (音量/角标映射用)
const volumeEntries = reactive(new Map()) // identity → 0-100 音量
const analysers = new Map() // identity → AnalyserNode
let meterRAF = null
let audioCtx = null
let meterSink = null // GainNode(0) → destination: 保 WebAudio 时钟推进, 不产生声音

function ensureAudioCtx() {
  if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)()
  return audioCtx
}

// 分析器不连 destination 时 WebAudio 渲染时钟不推进, getByteTimeDomainData 永远读成
// 静音 (音量条只有 1-10% 的噪声浮动就是这个现象)。挂一个 0 增益节点到 destination
// 保持时钟跑, 同时不产生任何声音。
function ensureMeterSink() {
  const ctx = ensureAudioCtx()
  if (!meterSink) {
    meterSink = ctx.createGain()
    meterSink.gain.value = 0
    meterSink.connect(ctx.destination)
  }
  return meterSink
}

// 统一 rAF 循环: 计算每个已注册 analyser 的 RMS 音量
function ensureMeterLoop() {
  if (meterRAF) return
  const data = new Uint8Array(512) // fftSize 1024 → frequencyBinCount 512
  const tick = () => {
    for (const [key, analyser] of analysers) {
      analyser.getByteTimeDomainData(data)
      let sum = 0
      for (let i = 0; i < data.length; i++) {
        const v = (data[i] - 128) / 128
        sum += v * v
      }
      const rms = Math.sqrt(sum / data.length)
      volumeEntries.set(key, Math.min(100, Math.round(rms * 260)))
    }
    meterRAF = requestAnimationFrame(tick)
  }
  meterRAF = requestAnimationFrame(tick)
}

function stopMeterLoop() {
  if (meterRAF) cancelAnimationFrame(meterRAF)
  meterRAF = null
  if (meterSink) {
    meterSink.disconnect()
    meterSink = null
  }
  analysers.clear()
  volumeEntries.clear()
}

// 为一个 MediaStream 注册音量分析 (key 用 identity; 本地用 localIdentity)
function registerVolume(key, stream) {
  if (!stream || !stream.getAudioTracks().length) return
  try {
    const ctx = ensureAudioCtx()
    // 无手势时 context 是 suspended 的, 分析器不更新; 尝试恢复 (失败无妨, 手势后 unlockAudio 会再 resume)
    if (ctx.state === 'suspended') ctx.resume().catch(() => {})
    const analyser = ctx.createAnalyser()
    analyser.fftSize = 1024
    ctx.createMediaStreamSource(stream).connect(analyser)
    analyser.connect(ensureMeterSink()) // 保时钟推进, 不产生声音
    analysers.set(key, analyser)
    ensureMeterLoop()
  } catch (e) {
    console.warn('[audio] 音量分析初始化失败:', e)
  }
}

function unregisterVolume(key) {
  analysers.delete(key)
  volumeEntries.set(key, 0)
}

// 本地麦克风音量: 等音轨发布就绪后注册 (发布是异步的, 轮询最多 1 秒)
function registerLocalVolume() {
  if (!room.value || !localIdentity.value) return
  let tries = 0
  const tryReg = () => {
    const pub = room.value.localParticipant.getTrackPublication(Track.Source.Microphone)
    if (pub && pub.track) {
      registerVolume(localIdentity.value, new MediaStream([pub.track.mediaStreamTrack]))
      return
    }
    if (++tries < 5) setTimeout(tryReg, 200)
  }
  tryReg()
}

// Toast messages
const toast = ref('')

let toastTimer = null

function showToast(msg) {
  toast.value = msg
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.value = '' }, 2500)
}

/**
 * Connect to a LiveKit room.
 * @param {string} token  — LiveKit access token (JWT)
 * @param {string} wsUrl  — LiveKit WebSocket URL
 * @param {string} myName — display name for self
 * @param {string} mode   — 'video' | 'voice'（语音模式不发布摄像头）
 */
async function connect(token, wsUrl, myName, mode = 'video') {
  if (isConnected.value || isConnecting.value) return

  const isVoice = mode === 'voice'
  isConnecting.value = true
  connectionError.value = ''
  trackEntries.clear()
  participantEntries.clear()

  try {
    const r = new Room({
      // 多人场景开启自适应码率与分层订阅, 控制带宽
      adaptiveStream: true,
      dynacast: true,
    })

    // ── Remote track subscribed → add to grid / participant list ──
    r.on(RoomEvent.TrackSubscribed, (track, _publication, participant) => {
      if (track.kind === 'video') {
        const entry = { track, name: participant.name || participant.identity }
        trackEntries.set(participant.identity, entry)
      } else if (track.kind === 'audio') {
        // 远端音频不会自动播放, 必须 attach 到 audio 元素, 否则没有声音
        const el = track.attach()
        audioElements.set(participant.identity, el)
        // 实时音量显示: 从 attach 元素的 MediaStream 接 AnalyserNode (device-test 同款)
        if (el.srcObject) registerVolume(participant.identity, el.srcObject)
        console.log('[audio] 远端音频已订阅:', participant.identity, participant.name)
        // 自动播放策略可能拦截: 记录状态, 手势解锁 (startAudio) + 轮询重试双路兜底
        el.play().then(
          () => { audioStatus.value = '远端音频播放中' },
          (e) => {
            console.warn('[audio] 自动播放被拦截:', e.name, e.message)
            audioStatus.value = '音频被浏览器拦截, 请点击屏幕任意位置解锁'
          },
        )
        // 轮询兜底: 部分 WebView 页面稳定后会自动解锁, 每秒重试直至播放成功 (最多 15 次)
        let tries = 0
        const poll = setInterval(() => {
          if (!el.paused) {
            clearInterval(poll)
            return
          }
          el.play().then(
            () => { audioStatus.value = '远端音频播放中'; clearInterval(poll) },
            () => {},
          )
          if (++tries >= 15) clearInterval(poll)
        }, 1000)
      }
      // 音频轨订阅也会触发本事件, 借此拿到远端参与者名字
      if (!participantEntries.has(participant.identity)) {
        participantEntries.set(participant.identity, {
          name: participant.name || participant.identity,
          speaking: false,
        })
      }
    })

    // ── Remote track unsubscribed → remove ──
    r.on(RoomEvent.TrackUnsubscribed, (track, _publication, participant) => {
      trackEntries.delete(participant.identity)
      const el = audioElements.get(participant.identity)
      if (el) {
        track.detach(el)
        audioElements.delete(participant.identity)
      }
      if (track.kind === 'audio') unregisterVolume(participant.identity)
      // 静音不离开房间, 不删参与者
    })

    // ── Participant joined (覆盖静音且未发轨的新人) ──
    r.on(RoomEvent.ParticipantConnected, (participant) => {
      participantEntries.set(participant.identity, {
        name: participant.name || participant.identity,
        speaking: false,
      })
    })

    // ── Participant left ──
    r.on(RoomEvent.ParticipantDisconnected, (participant) => {
      participantEntries.delete(participant.identity)
    })

    // ── 正在说话的人 (含本地) → 高亮 ──
    r.on(RoomEvent.ActiveSpeakersChanged, (speakers) => {
      const ids = new Set(speakers.map((p) => p.identity))
      for (const [id, entry] of participantEntries) {
        entry.speaking = ids.has(id)
      }
    })

    // ── Room disconnected ──
    r.on(RoomEvent.Disconnected, () => {
      isConnected.value = false
      showToast('连接已断开')
      releaseAudioResources() // 意外断开也要释放音频焦点
    })

    // ── 自动播放策略解锁: 浏览器/WebView 可能拦截远端音频,
    // 首次触摸/点击/按键时统一恢复全部已挂载的音频元素 (官方推荐 startAudio) ──
    const unlockAudio = () => {
      // AudioContext 可能被自动播放策略挂起, 手势时恢复 (音量条才有数据)
      if (audioCtx && audioCtx.state === 'suspended') audioCtx.resume()
      r.startAudio().then(
        () => { audioStatus.value = '远端音频播放中' },
        () => { audioStatus.value = '音频仍被拦截, 请再点击一次屏幕' },
      )
      unlockCleanup.forEach((fn) => fn())
      unlockCleanup = []
    }
    unlockCleanup = [
      () => document.removeEventListener('pointerdown', unlockAudio),
      () => document.removeEventListener('touchstart', unlockAudio),
      () => document.removeEventListener('keydown', unlockAudio),
    ]
    document.addEventListener('pointerdown', unlockAudio)
    document.addEventListener('touchstart', unlockAudio)
    document.addEventListener('keydown', unlockAudio)

    // ── Connect ──
    await r.connect(wsUrl, token)
    localIdentity.value = r.localParticipant.identity

    // ── Enable local mic ──
    await r.localParticipant.setMicrophoneEnabled(true)
    registerLocalVolume()
    participantEntries.set(r.localParticipant.identity, {
      name: myName + '（我）',
      speaking: false,
    })

    // ── Voice 房间严格纯语音: 绝不发布摄像头 (服务端 canPublishSources 会拒绝) ──
    if (!isVoice) {
      const camPub = await r.localParticipant.setCameraEnabled(true)
      // ── Add local tile ──
      if (camPub && camPub.track) {
        trackEntries.set('__local__', { track: camPub.track, name: myName + '（我）' })
      }
    }

    room.value = r
    isConnected.value = true

    showToast(isVoice ? '已加入语音房间' : '已加入房间')
  } catch (err) {
    console.error('LiveKit connect error:', err)
    connectionError.value = err.message || '连接失败'
    showToast('连接失败: ' + (err.message || '未知错误'))
  } finally {
    isConnecting.value = false
  }
}

/**
 * 手动解锁远端音频 (App WebView 自动播放被拦截时, 按钮/任意点击触发)。
 * App 端配置 mediaPlaybackRequiresUserGesture=false 后可免点击直接播放。
 */
async function unlockAudioNow() {
  if (!room.value) return
  if (audioCtx && audioCtx.state === 'suspended') audioCtx.resume()
  try {
    await room.value.startAudio()
    audioStatus.value = '远端音频播放中'
  } catch (e) {
    console.warn('[audio] startAudio 解锁失败:', e.name)
    audioStatus.value = '音频仍被拦截, 请再点击一次'
  }
}

// 彻底释放音频资源。Android WebView 的 WebRTC 音频会话结束时不彻底清理,
// 会持续占用系统音频焦点 → 手机静音/音量键失灵, 只能重启。必须:
// 停流 → 解绑 srcObject → 强制 load() → 移除元素 → 关闭 AudioContext。
function releaseAudioResources() {
  for (const el of audioElements.values()) {
    try {
      el.pause()
      el.srcObject = null
      el.removeAttribute('src')
      el.load() // 强制解除 WebRTC 流绑定, 释放底层音频渲染
      el.remove()
    } catch (e) { /* 元素可能已卸载 */ }
  }
  audioElements.clear()
  stopMeterLoop()
  if (audioCtx) {
    audioCtx.close().catch(() => {})
    audioCtx = null
    meterSink = null
  }
}

/**
 * Disconnect from the current room.
 */
async function disconnect() {
  try {
    if (room.value) {
      room.value.disconnect()
      room.value = null
    }
  } catch (e) {
    console.error('disconnect error:', e)
  }
  unlockCleanup.forEach((fn) => fn())
  unlockCleanup = []
  audioStatus.value = ''
  trackEntries.clear()
  participantEntries.clear()
  releaseAudioResources()
  localIdentity.value = ''
  isConnected.value = false
  isConnecting.value = false
}

/**
 * Toggle microphone on/off.
 */
async function toggleMic() {
  if (!room.value) return
  micOn.value = !micOn.value
  try {
    await room.value.localParticipant.setMicrophoneEnabled(micOn.value)
    if (micOn.value) registerLocalVolume()
    else unregisterVolume(localIdentity.value)
  } catch (e) {
    console.error('toggleMic error:', e)
    micOn.value = !micOn.value // rollback
  }
}

/**
 * Toggle camera on/off (video rooms only).
 */
async function toggleCam() {
  if (!room.value) return
  camOn.value = !camOn.value
  try {
    await room.value.localParticipant.setCameraEnabled(camOn.value)
  } catch (e) {
    console.error('toggleCam error:', e)
    camOn.value = !camOn.value // rollback
  }
}

/**
 * Clean up on page unload.
 */
function onBeforeUnload() {
  if (room.value) {
    room.value.disconnect()
    room.value = null
  }
  releaseAudioResources()
}

export function useLiveKit() {
  return {
    // state
    room,
    isConnected,
    isConnecting,
    connectionError,
    micOn,
    camOn,
    audioStatus,
    localIdentity,
    volumeEntries,
    trackEntries,
    participantEntries,
    toast,
    // actions
    connect,
    disconnect,
    toggleMic,
    toggleCam,
    unlockAudioNow,
    onBeforeUnload,
  }
}
