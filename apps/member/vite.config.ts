import path from 'node:path';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { VitePWA } from 'vite-plugin-pwa';

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      devOptions: { enabled: false },
      includeAssets: ['icons/icon-192.png', 'icons/icon-512.png', 'brand/*.png'],
      manifest: {
        name: 'NüHabit',
        short_name: 'NüHabit',
        description: 'NüHabit member app — credits, classes, bookings and QR gate access.',
        theme_color: '#f3ece2',
        background_color: '#f3ece2',
        display: 'standalone',
        start_url: '/',
        icons: [
          { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png' },
          {
            src: '/icons/icon-512-maskable.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,png,svg,ico,jpg}'],
        // The demo bundles the whole seed snapshot into the app shell, which
        // puts the main chunk above Workbox's 2 MiB precache default. Keep it
        // precached so the app (and its in-browser backend) works offline.
        maximumFileSizeToCacheInBytes: 5 * 1024 * 1024,
        navigateFallback: '/index.html',
        // /api is the backend (or, in the offline demo, the in-process mock).
        // Either way Workbox must never answer it with the app shell.
        navigateFallbackDenylist: [/^\/api\//],
        runtimeCaching: [
          // Fonts keep working offline after first load.
          {
            urlPattern: /^https:\/\/fonts\.googleapis\.com\/.*/,
            handler: 'StaleWhileRevalidate',
            options: { cacheName: 'google-fonts-styles' },
          },
          {
            urlPattern: /^https:\/\/fonts\.gstatic\.com\/.*/,
            handler: 'CacheFirst',
            options: {
              cacheName: 'google-fonts-files',
              expiration: { maxEntries: 20, maxAgeSeconds: 60 * 60 * 24 * 365 },
            },
          },
          // Stock photos keep working offline after first view.
          {
            urlPattern: /^https:\/\/images\.unsplash\.com\/.*/,
            handler: 'CacheFirst',
            options: {
              cacheName: 'stock-photos',
              expiration: { maxEntries: 60, maxAgeSeconds: 60 * 60 * 24 * 90 },
            },
          },
        ],
      },
    }),
  ],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  // The app calls /api on its own origin, so the browser never makes a
  // cross-origin request and CORS never enters the picture. In production
  // nginx routes /api to the Go backend; in development this proxy does the
  // same job, so `pnpm dev` talks to a local backend with nothing else to
  // configure. VITE_OFFLINE_DEMO=1 answers in-process instead and never
  // reaches the proxy at all.
  server: {
    port: Number(process.env.PORT) || 5173,
    proxy: { '/api': { target: process.env.API_PROXY_TARGET ?? 'http://localhost:8080', changeOrigin: true } },
  },
  preview: {
    port: Number(process.env.PORT) || 5173,
    proxy: { '/api': { target: process.env.API_PROXY_TARGET ?? 'http://localhost:8080', changeOrigin: true } },
  },
});
