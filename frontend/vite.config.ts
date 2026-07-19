import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 3000,
    strictPort: true,
    proxy: {
      '/api': {
        target: process.env.VITE_BACKEND_PROXY_TARGET ?? 'http://localhost:28080',
        changeOrigin: true,
      },
      '/ocr': {
        // Dev parity with production nginx.conf. The OCR service listens on
        // OCR_HOST_PORT (default 8000) when running locally outside Docker.
        target: process.env.VITE_OCR_PROXY_TARGET ?? 'http://localhost:18000',
        changeOrigin: true,
      },
    },
  },
})
