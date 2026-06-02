export function registerServiceWorker(): void {
  if (!('serviceWorker' in navigator) || import.meta.env.DEV) return

  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/public/dist/sw.js', { scope: '/' }).catch(() => {
      // Caching is an optimization; the app remains usable without a service worker.
    })
  })
}
