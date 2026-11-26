<!-- App.vue — root component with router view -->
<template>
  <router-view />
</template>

<script setup>
// App root — routing is handled by vue-router
</script>

<style>
/* ── Global Reset & Theme ── */
:root {
  --bg: #0d1117;
  --panel: #161b22;
  --border: rgba(255, 255, 255, 0.08);
  --text: #e6edf3;
  --muted: #8b949e;
  --accent: #3b82f6;
  --danger: #ef4444;
  --ok: #22c55e;
  --warn: #f59e0b;
}

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

html, body {
  height: 100%;
  overflow: hidden;
}

body {
  font-family: "Segoe UI", system-ui, -apple-system, sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
}

#app {
  height: 100%;
  display: flex;
  flex-direction: column;
}
</style>
