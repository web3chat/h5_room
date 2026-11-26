<!--
  ControlBar.vue — bottom control bar for mic/camera/hangup.
-->
<template>
  <div class="controls-bar">
    <button
      class="ctrl-btn"
      :class="{ active: micOn }"
      @click="$emit('toggleMic')"
    >
      <span class="icon">{{ micOn ? '🎤' : '🔇' }}</span>
      <span>麦克风</span>
    </button>
    <!-- 语音房间无摄像头按钮 -->
    <button
      v-if="showCam"
      class="ctrl-btn"
      :class="{ active: camOn }"
      @click="$emit('toggleCam')"
    >
      <span class="icon">{{ camOn ? '📷' : '📸' }}</span>
      <span>摄像头</span>
    </button>
    <button class="ctrl-btn danger" @click="$emit('hangup')">
      <span class="icon">📞</span>
      <span>挂断</span>
    </button>
  </div>
</template>

<script setup>
defineProps({
  micOn: { type: Boolean, default: true },
  camOn: { type: Boolean, default: true },
  showCam: { type: Boolean, default: true }, // false = 语音房间
})

defineEmits(['toggleMic', 'toggleCam', 'hangup'])
</script>

<style scoped>
.controls-bar {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 0.7rem 1rem;
  background: #161b22;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
  flex-wrap: wrap;
}
.ctrl-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  padding: 8px 14px;
  cursor: pointer;
  color: #e6edf3;
  font-size: 0.7rem;
  min-width: 60px;
  transition: background 0.15s;
  font-family: inherit;
}
.ctrl-btn:active {
  background: rgba(255, 255, 255, 0.12);
}
.ctrl-btn.active {
  background: #3b82f6;
  border-color: #3b82f6;
  color: #fff;
}
.ctrl-btn.danger {
  background: #ef4444;
  border-color: #ef4444;
  color: #fff;
}
.ctrl-btn .icon {
  font-size: 1.3rem;
  line-height: 1;
}
.ctrl-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
</style>
