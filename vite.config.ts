import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  base: '/public/dist/',
  publicDir: false,
  plugins: [
    // <cap-widget> is a custom element defined by the Cap widget script, not a
    // Vue component. Without this the compiler tries to resolve it and fails.
    vue({
      template: {
        compilerOptions: {
          isCustomElement: tag => tag === 'cap-widget'
        }
      }
    }),
    VitePWA({
      registerType: 'autoUpdate',
      injectRegister: false,
      filename: 'sw.js',
      manifest: false,
      workbox: {
        cleanupOutdatedCaches: true,
        navigateFallbackDenylist: [/^\//],
        globPatterns: ['assets/*.{js,css,woff,woff2,svg,png,jpg,jpeg,gif,webp}'],
        runtimeCaching: [
          {
            urlPattern: ({ request }) => request.destination === 'script' || request.destination === 'style' || request.destination === 'font',
            handler: 'CacheFirst',
            options: {
              cacheName: 'easyimage-static',
              expiration: {
                maxEntries: 120,
                maxAgeSeconds: 60 * 60 * 24 * 365
              },
              cacheableResponse: {
                statuses: [0, 200]
              }
            }
          },
          {
            urlPattern: ({ request, url }) => request.destination === 'image' && url.pathname === '/app/thumb',
            handler: 'CacheFirst',
            options: {
              cacheName: 'easyimage-thumbnails',
              expiration: {
                maxEntries: 300,
                maxAgeSeconds: 60 * 60 * 24 * 14,
                purgeOnQuotaError: true
              },
              cacheableResponse: {
                statuses: [0, 200]
              }
            }
          },
          {
            urlPattern: ({ url }) => url.pathname === '/api/list',
            handler: 'StaleWhileRevalidate',
            options: {
              cacheName: 'easyimage-gallery-api',
              expiration: {
                maxEntries: 80,
                maxAgeSeconds: 60 * 60
              },
              cacheableResponse: {
                statuses: [0, 200]
              }
            }
          }
        ]
      }
    })
  ],
  build: {
    outDir: 'public/dist',
    emptyOutDir: true,
    manifest: true,
    rollupOptions: {
      input: {
        upload: 'web/src/entries/upload.ts',
        gallery: 'web/src/entries/gallery.ts',
        admin: 'web/src/entries/admin.ts'
      }
    }
  }
})
