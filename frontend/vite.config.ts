import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import svgLoader from 'vite-svg-loader'

const backendUrl = process.env.BACKEND_URL || `http://localhost:${process.env.BACKEND_PORT || '8080'}`

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    svgLoader(),
  ],
  resolve: {
    alias: {
      crypto: 'crypto-browserify',
    },
  },
  server: {
    proxy: {
      '/api': {
        target: backendUrl,
        changeOrigin: true,
      },
      '/images': {
        target: backendUrl,
        changeOrigin: true,
      },
    },
  },
})