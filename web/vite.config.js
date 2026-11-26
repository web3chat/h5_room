import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    // 语法降级到 ES2018: 兼容小米等旧 WebView (livekit-client 内部用了可选链等现代语法,
    // 不降级的话旧内核解析报错 → 整个应用黑屏, 见 device-test.html 排查过程)
    target: 'es2018',
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:3000',
        changeOrigin: true,
      },
    },
  },
})
