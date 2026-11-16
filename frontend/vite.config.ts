import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  // noVNC is plain ESM with a top-level await; serve it as is in dev instead
  // of pre-bundling it.
  optimizeDeps: {
    exclude: ['@novnc/novnc'],
  },
  build: {
    chunkSizeWarningLimit: 6000,
    target: 'es2022',
  },
  worker: {
    format: 'es',
  },
})
