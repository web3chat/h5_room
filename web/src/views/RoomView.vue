<!--
  RoomView.vue — multi-party video / voice room (video ≤5, voice ≤10).
  Shows video tiles (video mode) or participant list (voice mode), status bar, controls.
-->
<template>
  <div class="room-panel">
    <!-- Top Status Bar -->
    <div class="topbar">
      <div class="room-info">
        <span class="status-dot" :class="statusClass"></span>
        <span class="room-name">{{ roomName }}</span>
        <span v-if="isVoice" class="mode-tag">语音</span>
        <span class="participant-count">{{ participantCount }}</span>
      </div>
      <span class="my-name">{{ myName }}</span>
    </div>

    <!-- Loading overlay -->
    <div v-if="isConnecting" class="loading-overlay">
      <div class="loading-spinner"></div>
      <p style="color:#8b949e;margin-top:1rem;">正在连接…</p>
    </div>

    <!-- 语音房间: 参与者列表; 视频房间: 视频网格 -->
    <ParticipantList v-if="isVoice" :entries="participantList" :volumes="volumeEntries" />
    <VideoGrid v-else :entries="trackList" :volumes="volumeEntries" :localIdentity="localIdentity" />

    <!-- Control Bar -->
    <ControlBar
      :micOn="micOn"
      :camOn="camOn"
      :showCam="!isVoice"
      @toggleMic="toggleMic"
      @toggleCam="toggleCam"
      @hangup="handleHangup"
    />

    <!-- Error banner -->
    <div v-if="connectionError && !isConnected" class="error-banner">
      {{ connectionError }}
      <button class="btn-back" @click="handleHangup">返回</button>
    </div>

    <!-- 音频播放状态诊断 (排查自动播放策略) -->
    <div v-if="audioStatus" class="audio-status">
      {{ audioStatus }}
      <button v-if="audioStatus !== '远端音频播放中'" class="audio-unlock" @click="unlockAudioNow">
        🔊 开启声音
      </button>
    </div>

    <!-- Toast -->
    <Toast :message="toast" />
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useLiveKit } from '../composables/useLiveKit.js'
import VideoGrid from '../components/VideoGrid.vue'
import ParticipantList from '../components/ParticipantList.vue'
import ControlBar from '../components/ControlBar.vue'
import Toast from '../components/Toast.vue'

const route = useRoute()
const router = useRouter()

const {
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
  connect,
  disconnect,
  toggleMic,
  toggleCam,
  unlockAudioNow,
  onBeforeUnload,
} = useLiveKit()

// ── Params from route query (passed by JoinView) ──
const roomName = computed(() => route.query.room || '')
const myName = computed(() => route.query.name || '')
const token = computed(() => route.query.token || '')
const wsUrl = computed(() => route.query.ws_url || '')

// ── 房间模式: 链接参数为准 (非 voice 一律 video) ──
const mode = computed(() => (route.query.mode === 'voice' ? 'voice' : 'video'))
const isVoice = computed(() => mode.value === 'voice')

// ── Derived ──
const statusClass = computed(() => {
  if (isConnected.value) return 'connected'
  if (isConnecting.value) return 'connecting'
  return 'disconnected'
})

const participantCount = computed(() => {
  if (isVoice.value) {
    return `${participantEntries.size} 人`
  }
  const local = trackEntries.get('__local__') ? 1 : 0
  const remotes = [...trackEntries.keys()].filter(k => k !== '__local__').length
  return `${local + remotes} 人`
})

// Convert Map to array for VideoGrid / ParticipantList
const trackList = computed(() => [...trackEntries.entries()])
const participantList = computed(() => [...participantEntries.entries()])

// ── Connect on mount ──
onMounted(async () => {
  if (!token.value) {
    router.replace('/')
    return
  }
  await connect(token.value, wsUrl.value, myName.value, mode.value)
})

// ── Hangup → back to join ──
async function handleHangup() {
  await disconnect()
  router.replace('/')
}

// ── Keyboard shortcuts (v/V 仅视频房间) ──
function onKeyDown(e) {
  if (e.target.closest('input')) return
  if (e.key === 'm' || e.key === 'M') toggleMic()
  if (!isVoice.value && (e.key === 'v' || e.key === 'V')) toggleCam()
}

onMounted(() => document.addEventListener('keydown', onKeyDown))
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeyDown)
  onBeforeUnload()
})
</script>

<style scoped>
.room-panel {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.6rem 1rem;
  background: #161b22;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}
.room-info {
  display: flex;
  align-items: center;
  gap: 8px;
}
.room-name {
  font-weight: 600;
  font-size: 0.95rem;
}
.participant-count {
  font-size: 0.78rem;
  color: #8b949e;
  background: rgba(255, 255, 255, 0.06);
  padding: 2px 8px;
  border-radius: 10px;
}
.mode-tag {
  font-size: 0.72rem;
  color: #fff;
  background: #8b5cf6;
  padding: 2px 8px;
  border-radius: 10px;
}
.my-name {
  font-size: 0.78rem;
  color: #8b949e;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}
.status-dot.connected { background: #22c55e; }
.status-dot.disconnected { background: #ef4444; }
.status-dot.connecting {
  background: #f59e0b;
  animation: pulse 1s infinite;
}
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.loading-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  z-index: 50;
}
.loading-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid rgba(255, 255, 255, 0.2);
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.error-banner {
  position: absolute;
  bottom: 100px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(239, 68, 68, 0.9);
  color: #fff;
  padding: 0.75rem 1.5rem;
  border-radius: 12px;
  font-size: 0.85rem;
  z-index: 80;
  display: flex;
  align-items: center;
  gap: 12px;
}
.btn-back {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 4px 12px;
  cursor: pointer;
  font-size: 0.8rem;
  font-family: inherit;
}

.audio-status {
  position: fixed;
  bottom: 92px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(0, 0, 0, 0.75);
  color: #93c5fd;
  font-size: 0.78rem;
  padding: 5px 12px;
  border-radius: 10px;
  z-index: 70;
  max-width: 90vw;
  text-align: center;
  display: flex;
  align-items: center;
  gap: 8px;
}
.audio-unlock {
  background: #3b82f6;
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: 5px 12px;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  font-family: inherit;
  white-space: nowrap;
}
</style>
