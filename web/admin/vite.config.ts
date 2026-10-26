import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:18610', changeOrigin: false },
      '/healthz': { target: 'http://127.0.0.1:18610', changeOrigin: false },
    },
  },
});
