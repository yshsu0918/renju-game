import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// In development, Vite serves the app and proxies /ws to the Go server, so the
// WebSocket is same-origin. In production the Go server serves dist/ itself.
export default defineConfig({
  plugins: [vue()],
  server: {
    host: true,
    proxy: {
      '/ws': { target: process.env.RENJU_SERVER ?? 'http://localhost:8200', ws: true },
    },
  },
  test: {
    environment: 'jsdom',
  },
})
