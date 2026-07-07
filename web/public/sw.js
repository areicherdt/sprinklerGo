// Minimaler Service Worker: App-Shell und Assets aus dem Cache, API immer
// über das Netz. Bei Offline-Navigation fällt die Shell aus dem Cache zurück
// (Status-Daten brauchen den Server).
const CACHE = 'sprinklergo-shell-v1'
const SHELL = ['/', '/manifest.webmanifest', '/sprinkler.svg']

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)))
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url)
  if (event.request.method !== 'GET' || url.pathname.startsWith('/api/')) {
    return // API und Mutationen: immer Netz
  }
  if (url.pathname.startsWith('/assets/')) {
    // Gehashte Build-Assets: cache-first
    event.respondWith(
      caches.open(CACHE).then(async (cache) => {
        const hit = await cache.match(event.request)
        if (hit) return hit
        const res = await fetch(event.request)
        if (res.ok) cache.put(event.request, res.clone())
        return res
      }),
    )
    return
  }
  // Navigation & Shell: network-first mit Cache-Fallback
  event.respondWith(
    fetch(event.request)
      .then((res) => {
        if (res.ok && event.request.mode === 'navigate') {
          const copy = res.clone()
          caches.open(CACHE).then((c) => c.put('/', copy))
        }
        return res
      })
      .catch(() => caches.match(event.request.mode === 'navigate' ? '/' : event.request)),
  )
})
