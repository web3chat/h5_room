<!--
  ParticipantList.vue — 语音房间参与者列表: 头像 + 名字 + 说话光圈。
  纯展示组件, 数据来自 useLiveKit 的 participantEntries。
-->
<template>
  <div class="participant-list">
    <div
      v-for="[id, p] in entries"
      :key="id"
      class="participant-card"
      :class="{ speaking: p.speaking }"
    >
      <div class="avatar">{{ initialOf(p.name) }}</div>
      <span class="name">{{ p.name }}</span>
      <!-- 实时音量条 (device-test.html 同款渐变) -->
      <div class="vol-bar">
        <div class="vol-fill" :style="{ width: (volumes.get(id) || 0) + '%' }"></div>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  entries: { type: Array, default: () => [] }, // [[id, {name, speaking}], ...]
  volumes: { type: Object, default: () => new Map() }, // identity → 0-100 实时音量
})

function initialOf(name) {
  const n = (name || '?').trim()
  return n.length > 0 ? n[0].toUpperCase() : '?'
}
</script>

<style scoped>
.participant-list {
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  align-content: center;
  justify-content: center;
  gap: 18px;
  padding: 16px;
  overflow-y: auto;
}

.participant-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 18px 20px;
  border-radius: 16px;
  background: #161b22;
  border: 1px solid rgba(255, 255, 255, 0.08);
  min-width: 110px;
}

.avatar {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.5rem;
  font-weight: 600;
  color: #fff;
  background: linear-gradient(135deg, #3b82f6, #8b5cf6);
  user-select: none;
}

.name {
  font-size: 0.85rem;
  color: #e6edf3;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 实时音量条 */
.vol-bar {
  width: 90px;
  height: 6px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.12);
  overflow: hidden;
}
.vol-fill {
  height: 100%;
  width: 0%;
  background: linear-gradient(90deg, #22c55e, #eab308, #ef4444);
  transition: width 0.08s linear;
}

/* 说话时绿色呼吸光圈 */
.participant-card.speaking .avatar {
  box-shadow: 0 0 0 4px var(--ok);
  animation: speak-pulse 1.2s ease-in-out infinite;
}

@keyframes speak-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 3px var(--ok);
  }
  50% {
    box-shadow: 0 0 0 8px transparent;
  }
}
</style>
