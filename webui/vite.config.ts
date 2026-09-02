import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [solid(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    // 后端端口可注入：默认 20128，与网关默认启动端口一致
    // 用法：GW_PORT=20128 npm run dev
    proxy: {
      '/api': `http://127.0.0.1:${process.env.GW_PORT ?? 20128}`,
      '/v1': `http://127.0.0.1:${process.env.GW_PORT ?? 20128}`,
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
