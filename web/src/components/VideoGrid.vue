<!--
  VideoGrid.vue — grid layout for video tiles. Supports 1-5 participants.
-->
<template>
  <div class="video-grid" :class="`cols-${colClass}`">
    <template v-if="entries.length > 0">
      <VideoTile
        v-for="[id, entry] in entries"
        :key="id"
        :track="entry.track"
        :name="entry.name"
        :muted="id === '__local__'"
        :volume="id === '__local__' ? (volumes.get(localIdentity) || 0) : (volumes.get(id) || 0)"
      />
    </template>
    <VideoTile v-else :track="null" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import VideoTile from './VideoTile.vue'

const props = defineProps({
  entries: { type: Array, default: () => [] }, // [[id, {track, name}], ...]
  volumes: { type: Object, default: () => new Map() }, // identity → 0-100 实时音量
  localIdentity: { type: String, default: '' }, // 本地 identity (本地格子 '__local__' 的音量映射)
})

const count = computed(() => props.entries.length)

const colClass = computed(() => {
  if (count.value <= 1) return 1
  if (count.value <= 4) return 2
  return 3 // 5 人: 3+2 布局
})
</script>

<style scoped>
.video-grid {
  flex: 1;
  display: grid;
  gap: 8px;
  padding: 8px;
  overflow-y: auto;
  align-content: center;
  background: #000;
}
.video-grid.cols-1 {
  grid-template-columns: 1fr;
}
.video-grid.cols-2 {
  grid-template-columns: repeat(2, 1fr);
}
.video-grid.cols-3 {
  grid-template-columns: repeat(3, 1fr);
}
</style>
