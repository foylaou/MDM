import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    allowedHosts: [
      'mdm.isha.net'
    ],
    proxy: {
      '/mdm.v1': {
        target: 'http://localhost:8080',
        // SSE / streaming 需要長連線，不要超時
        timeout: 0,
      },
      '/webhook': 'http://localhost:8080',
      '/api': 'http://localhost:8080',
    },
  },
})
