<!-- Toast.vue — floating toast message -->
<template>
  <div class="toast" :class="{ show: !!message }">{{ message }}</div>
</template>

<script setup>
defineProps({
  message: { type: String, default: '' },
})
</script>

<style scoped>
.toast {
  position: fixed;
  bottom: 100px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(0, 0, 0, 0.85);
  color: #fff;
  padding: 0.5rem 1.2rem;
  border-radius: 20px;
  font-size: 0.82rem;
  z-index: 100;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.2s;
}
.toast.show {
  opacity: 1;
}
</style>
