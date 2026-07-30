import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    // Keeps the browser on one origin so cookies behave in development.
    // The port follows the backend's PORT so running on a non-default port does
    // not silently proxy to a stale server on 8080.
    proxy: {
      '/api': {
        target: `http://localhost:${process.env.BACKEND_PORT ?? process.env.PORT ?? 8080}`,
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ''),
      },
    },
  },
})
