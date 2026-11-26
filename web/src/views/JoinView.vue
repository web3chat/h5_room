<!--
  JoinView.vue — room join form with room/pin/name inputs.
  Supports URL parameter pre-fill: ?room=xxx&pin=xxx&name=xxx&mode=voice
-->
<template>
  <div class="join-panel">
    <div class="join-card">
      <h1>🎥 加入音视频房间</h1>
      <p class="sub">{{ isVoice ? '语音房间，仅麦克风，多人实时对话' : '输入房间信息，开始多人视频通话（最多 5 人）' }}</p>

      <div class="field">
        <label>房间号</label>
        <input
          type="text"
          v-model="room"
          placeholder="例如 room001"
          maxlength="48"
          @keydown.enter="handleJoin"
        />
      </div>

      <div class="field">
        <label>入会口令</label>
        <input
          type="text"
          v-model="pin"
          placeholder="房间口令"
          maxlength="32"
          @keydown.enter="handleJoin"
        />
      </div>

      <div class="field">
        <label>你的昵称</label>
        <input
          type="text"
          v-model="name"
          placeholder="输入昵称"
          maxlength="32"
          @keydown.enter="handleJoin"
        />
      </div>

      <!-- 钱包地址: 进房身份, 服务端校验与建房绑定的 user_addr 一致才放行 -->
      <div class="field">
        <label>钱包地址</label>
        <div v-if="userAddr" class="wallet-addr">🔗 {{ shortAddr }}</div>
        <button
          v-else
          class="btn-wallet"
          :disabled="walletLoading"
          @click="connectWallet"
        >
          {{ walletLoading ? '连接中…' : '🔑 登录获取钱包地址' }}
        </button>
        <div v-if="walletHint" class="err-msg">{{ walletHint }}</div>
      </div>

      <button
        class="btn btn-accent btn-full"
        :disabled="joining"
        @click="handleJoin"
      >
        {{ joining ? '加入中…' : '加入房间' }}
      </button>

      <!-- 服务端生成随机房间号+PIN (App 端也可直接调 POST /api/v1/room/create) -->
      <button
        class="gen-link"
        :disabled="creating"
        @click="handleCreateRoom"
      >
        {{ creating ? '生成中…' : '✨ 生成房间链接' }}
      </button>
      <div v-if="genMsg" class="gen-msg">{{ genMsg }}</div>

      <!-- 复制房间链接: webview 不支持时引导去外部浏览器 -->
      <button
        class="copy-link"
        :disabled="!room.trim()"
        @click="copyRoomLink"
      >
        {{ copied ? '已复制 ✓' : '🔗 复制房间链接' }}
      </button>

      <div v-if="error" class="err-msg">{{ error }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { fetchToken, createRoom } from '../api/index.js'

const router = useRouter()
const route = useRoute()

const room = ref('')
const pin = ref('')
const name = ref('')
const mode = ref('video')
// 进房者 0x 地址: 由钱包登录获取 (App WebView bridge 或浏览器扩展钱包),
// 不信任链接参数 —— 房间绑定了 user_addr 时服务端校验与此一致
const userAddr = ref('')
const walletLoading = ref(false)
const walletHint = ref('')
const joining = ref(false)
const creating = ref(false)
const genMsg = ref('')
const error = ref('')
const copied = ref(false)

const isVoice = computed(() => mode.value === 'voice')
const shortAddr = computed(() => {
  const a = userAddr.value
  return a ? a.slice(0, 6) + '…' + a.slice(-4) : ''
})

// Read URL query params to pre-fill form (from both route.query and window.location.search
// for compatibility with direct links like https://host/?room=xxx&pin=xxx&mode=voice)
onMounted(() => {
  const q = new URLSearchParams(window.location.search)
  const fromRoute = (key) => route.query[key] || ''
  if (fromRoute('room') || q.get('room')) room.value = fromRoute('room') || q.get('room')
  if (fromRoute('pin') || q.get('pin')) pin.value = fromRoute('pin') || q.get('pin')
  if (fromRoute('name') || q.get('name')) name.value = fromRoute('name') || q.get('name')
  // mode 只预填, 不进自动加入条件; 最终以服务端返回为准
  const m = (fromRoute('mode') || q.get('mode') || '').toLowerCase()
  if (m === 'voice') mode.value = 'voice'

  // Auto-join if all params present and user is clicking from App link
  if (room.value && pin.value && name.value) {
    // Only auto-join if we have explicit URL params (not just manual input)
    const hasUrlParams = !!(fromRoute('room') || q.get('room'))
    if (hasUrlParams) {
      setTimeout(handleJoin, 300)
    }
  }
})

async function handleJoin() {
  error.value = ''

  const r = room.value.trim()
  const p = pin.value.trim()
  const n = name.value.trim()

  if (!r || r.length < 4) {
    error.value = '房间号至少 4 位'
    return
  }
  if (!/^[a-z0-9_]+$/.test(r)) {
    error.value = '房间号只能包含小写字母、数字、下划线'
    return
  }
  if (!p) {
    error.value = '请输入口令'
    return
  }
  if (!n) {
    error.value = '请输入昵称'
    return
  }

  // 钱包身份: 房间未绑定 user_addr 时服务端不校验, 无需钱包即可进房;
  // 先不带地址请求 token, 仅当服务端返回 user address not allowed 时再连钱包重试
  // (App 内 bridge 自动提供地址; 浏览器则触发钱包扩展弹窗)

  // 进房前真实探测设备: 能准确识别 webview 权限限制 (Android onPermissionRequest 未放行等)。
  // 探测完立即释放, 避免占 LiveKit 房间名额却拿不到设备 (幽灵参与者)
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    error.value = '当前环境不支持摄像头/麦克风，请使用 Chrome/Edge 并通过 HTTPS 访问'
    return
  }
  joining.value = true
  try {
    const probe = await navigator.mediaDevices.getUserMedia(
      isVoice.value ? { audio: true } : { audio: true, video: true },
    )
    probe.getTracks().forEach((t) => t.stop())
  } catch (e) {
    joining.value = false
    error.value = isVoice.value
      ? '无法获取麦克风：请确认已授权。如在 App 内打开，请复制下方链接到浏览器'
      : '无法获取摄像头/麦克风：请确认已授权。如在 App 内打开，请复制下方链接到浏览器'
    return
  }

  try {
    const tokenParams = {
      room: r,
      name: n, // 只传昵称; identity 由后端生成, 避免同昵称用户互踢
      join_pin: p,
      mode: mode.value,
    }
    let resp = await fetchToken({ ...tokenParams, user_addr: userAddr.value || undefined })

    // 未绑定用户的房间直接放行; 绑定房间才要求钱包地址, 连接后重试一次
    if (resp.code !== 0 && !userAddr.value && /user address not allowed/i.test(resp.error || '')) {
      await connectWallet()
      if (!userAddr.value) {
        throw new Error(walletHint.value || '该房间需要钱包身份，请先登录获取钱包地址')
      }
      resp = await fetchToken({ ...tokenParams, user_addr: userAddr.value })
    }

    if (resp.code !== 0) {
      throw new Error(resp.error || '获取 token 失败')
    }

    // Navigate to room view with connection params
    router.push({
      path: '/room',
      query: {
        room: r,
        token: resp.token,
        ws_url: resp.ws_url || '',
        name: n,
        is_owner: resp.is_owner ? '1' : '0',
        mode: resp.mode || 'video', // 服务端权威值 (旧后端无 mode 时兜底 video)
      },
    })
  } catch (err) {
    console.error('Join error:', err)
    error.value = err.message || '加入失败，请重试'
  } finally {
    joining.value = false
  }
}

// 获取钱包地址: 优先 App WebView bridge (App 已登录注入), 其次浏览器扩展钱包 (EIP-1193)。
// App 对接约定: 加载页面时注入 window.H5Wallet.getAddress() → Promise<0x地址>
async function getWalletAddress() {
  const w = window.H5Wallet
  if (w && typeof w.getAddress === 'function') {
    try {
      const addr = await w.getAddress()
      if (addr) return String(addr).trim()
    } catch (e) {
      console.warn('H5Wallet bridge error:', e)
    }
  }
  if (window.ethereum && typeof window.ethereum.request === 'function') {
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' })
    if (Array.isArray(accounts) && accounts[0]) return String(accounts[0]).trim()
  }
  return ''
}

// 登录获取钱包地址 (按钮触发 / 进房自动触发)
async function connectWallet() {
  walletHint.value = ''
  walletLoading.value = true
  try {
    const addr = await getWalletAddress()
    if (addr) {
      userAddr.value = addr.toLowerCase()
    } else {
      walletHint.value = '未检测到钱包：如在 App 内打开请确认已登录；否则请安装 MetaMask 等钱包扩展后刷新重试'
    }
  } catch (e) {
    console.error('Wallet connect error:', e)
    walletHint.value = '连接钱包失败：' + (e.message || e.code || '未知错误')
  } finally {
    walletLoading.value = false
  }
}

// 服务端创建房间: 随机房间号+PIN, 自动填入表单并复制链接
async function handleCreateRoom() {
  genMsg.value = ''
  error.value = ''
  creating.value = true
  try {
    const resp = await createRoom(mode.value)
    if (resp.code !== 0) {
      throw new Error(resp.error || '创建房间失败')
    }
    room.value = resp.room
    pin.value = resp.pin
    mode.value = resp.mode || mode.value
    await copyRoomLink()
    genMsg.value = `已创建房间 ${resp.room}，链接已复制 ✓（口令 ${resp.pin}）`
  } catch (err) {
    console.error('Create room error:', err)
    error.value = err.message || '创建失败，请重试'
  } finally {
    creating.value = false
  }
}

// 复制房间链接 (room+pin+mode; 昵称已填则带上 name —— 对方打开链接自动填入, 无需手输)
async function copyRoomLink() {
  const r = room.value.trim()
  if (!r) return
  const p = pin.value.trim()
  const n = name.value.trim()
  const url =
    location.origin + location.pathname +
    `?room=${encodeURIComponent(r)}` +
    (p ? `&pin=${encodeURIComponent(p)}` : '') +
    `&mode=${mode.value}` +
    (n ? `&name=${encodeURIComponent(n)}` : '')
  try {
    await navigator.clipboard.writeText(url)
  } catch (e) {
    // webview 无 clipboard API 时降级 execCommand
    const ta = document.createElement('textarea')
    ta.value = url
    ta.style.cssText = 'position:fixed;opacity:0'
    document.body.appendChild(ta)
    ta.select()
    try { document.execCommand('copy') } catch (_) {}
    document.body.removeChild(ta)
  }
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}
</script>

<style scoped>
.join-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  padding: 2rem 1rem;
}
.join-card {
  background: #161b22;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  padding: 2rem 1.5rem;
  width: 100%;
  max-width: 400px;
}
.join-card h1 {
  font-size: 1.35rem;
  font-weight: 700;
  margin: 0 0 0.25rem;
}
.join-card .sub {
  color: #8b949e;
  font-size: 0.85rem;
  margin-bottom: 1.5rem;
}
.field {
  margin-bottom: 1rem;
}
.field label {
  display: block;
  font-size: 0.8rem;
  color: #8b949e;
  margin-bottom: 4px;
  font-weight: 500;
}
.field input {
  width: 100%;
  padding: 0.6rem 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.3);
  color: #e6edf3;
  font-size: 0.95rem;
  box-sizing: border-box;
}
.field input:focus {
  outline: 2px solid #3b82f6;
  outline-offset: 2px;
}
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0.65rem 1.25rem;
  border-radius: 10px;
  border: none;
  font-size: 0.95rem;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.15s;
  font-family: inherit;
}
.btn:active { opacity: 0.8; }
.btn-full { width: 100%; }
.btn-accent { background: #3b82f6; color: #fff; }
.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.err-msg {
  color: #ef4444;
  font-size: 0.82rem;
  margin-top: 0.5rem;
}
.wallet-addr {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0.6rem 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.3);
  color: #e6edf3;
  font-size: 0.9rem;
  font-family: 'SF Mono', Consolas, monospace;
}
.btn-wallet {
  width: 100%;
  padding: 0.55rem 1.25rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.25);
  background: transparent;
  color: #8b949e;
  font-size: 0.85rem;
  cursor: pointer;
  font-family: inherit;
  transition: color 0.15s, border-color 0.15s;
}
.btn-wallet:hover:not(:disabled) {
  color: #e6edf3;
  border-color: #3b82f6;
}
.btn-wallet:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.gen-link {
  width: 100%;
  margin-top: 10px;
  padding: 0.55rem 1.25rem;
  border-radius: 10px;
  border: 1px solid rgba(59, 130, 246, 0.5);
  background: rgba(59, 130, 246, 0.08);
  color: #93c5fd;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
  font-family: inherit;
  transition: color 0.15s, background 0.15s, border-color 0.15s;
}
.gen-link:hover:not(:disabled) {
  color: #fff;
  background: rgba(59, 130, 246, 0.18);
  border-color: #3b82f6;
}
.gen-link:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.gen-msg {
  color: #4ade80;
  font-size: 0.8rem;
  margin-top: 8px;
  text-align: center;
  word-break: break-all;
}
.copy-link {
  width: 100%;
  margin-top: 10px;
  padding: 0.55rem 1.25rem;
  border-radius: 10px;
  border: 1px dashed rgba(255, 255, 255, 0.25);
  background: transparent;
  color: #8b949e;
  font-size: 0.85rem;
  cursor: pointer;
  font-family: inherit;
  transition: color 0.15s, border-color 0.15s;
}
.copy-link:hover:not(:disabled) {
  color: #e6edf3;
  border-color: #3b82f6;
}
.copy-link:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
