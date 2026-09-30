import { defineConfig } from 'vite'
import { resolve } from 'node:path'

// Two entry pages. Go serves index.html at "/" and room.html at "/room/{id}".
// Output goes straight into the Go package so //go:embed picks it up.
export default defineConfig({
  build: {
    outDir: '../internal/server/dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: resolve(__dirname, 'index.html'),
        room: resolve(__dirname, 'room.html'),
      },
    },
  },
  server: {
    port: 5173,
    // In dev, Vite serves the pages with hot reload and forwards API and
    // WebSocket traffic to the Go server (`make run` in another terminal).
    proxy: {
      '/api': 'http://localhost:8070',
      '/healthz': 'http://localhost:8070',
      '/ws': { target: 'ws://localhost:8070', ws: true },
    },
  },
  // Vite's dev server has no idea /room/<id> means room.html; rewrite it.
  plugins: [
    {
      name: 'room-rewrite',
      configureServer(server) {
        server.middlewares.use((req, _res, next) => {
          if (req.url?.startsWith('/room/')) req.url = '/room.html'
          next()
        })
      },
    },
  ],
})