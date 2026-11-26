<!--
  VideoTile.vue — single video tile that displays a LiveKit video track.
-->
<template>
  <div class="video-tile" :class="{ empty: !track }">
    <video
      v-if="track"
      ref="videoEl"
      autoplay
      playsinline
      :muted="muted"
    ></video>
    <span v-if="!track" class="placeholder">等待加入…</span>
    <span v-if="name" class="tile-label">{{ name }}</span>
    <!-- 实时音量条 (device-test.html 同款: 绿→黄→红渐变, 音量 0-100) -->
    <div v-if="track" class="vol-bar">
      <div class="vol-fill" :style="{ width: volume + '%' }"></div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps({
  track: { type: Object, default: null },
  name: { type: String, default: '' },
  muted: { type: Boolean, default: false },
  volume: { type: Number, default: 0 }, // 实时音量 0-100 (rAF 计算 RMS)
})

const videoEl = ref(null)

function attachTrack() {
  if (props.track && videoEl.value) {
    props.track.attach(videoEl.value)
  }
}

function detachTrack() {
  if (props.track && videoEl.value) {
    props.track.detach(videoEl.value)
  }
}

onMounted(attachTrack)

watch(() => props.track, (newTrack, oldTrack) => {
  if (oldTrack && videoEl.value) {
    oldTrack.detach(videoEl.value)
  }
  if (newTrack && videoEl.value) {
    newTrack.attach(videoEl.value)
  }
})

onBeforeUnmount(detachTrack)
</script>

<style scoped>
.video-tile {
  position: relative;
  background: #1a1a2e;
  border-radius: 10px;
  overflow: hidden;
  aspect-ratio: 4 / 3;
  min-height: 160px;
}
.video-tile video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  background: #0a0a1a;
}
.video-tile .tile-label {
  position: absolute;
  bottom: 6px;
  left: 6px;
  background: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: 0.75rem;
  padding: 2px 8px;
  border-radius: 6px;
  max-width: calc(100% - 12px);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
}
/* 实时音量条 */
.vol-bar {
  position: absolute;
  left: 8px;
  right: 8px;
  bottom: 32px;
  height: 6px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.15);
  overflow: hidden;
  pointer-events: none;
}
.vol-fill {
  height: 100%;
  width: 0%;
  background: linear-gradient(90deg, #22c55e, #eab308, #ef4444);
  transition: width 0.08s linear;
}
.video-tile.empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #8b949e;
  font-size: 0.85rem;
}
.video-tile .placeholder {
  color: #8b949e;
  font-size: 0.85rem;
}
</style>
