import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The build lands in the Go package that embeds it (//go:embed all:dist).
export default defineConfig({
  base: '/admin/',
  plugins: [vue()],
  build: {
    outDir: '../internal/adminui/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    // `npm run dev` talks to the Go service on :3000 (run it with ADMIN_ENABLED=true ADMIN_PANEL_ENABLED=true)
    proxy: {
      '/admin/config.json': 'http://localhost:3000',
      '/api': 'http://localhost:3000',
    },
  },
})
