const CACHE = 'com-itemgather-static-v1'
self.addEventListener('install', event => { event.waitUntil(caches.open(CACHE).then(cache => cache.addAll(['/','/manifest.webmanifest','/icon.svg'])).then(() => self.skipWaiting())) })
self.addEventListener('activate', event => { event.waitUntil(caches.keys().then(keys => Promise.all(keys.filter(key => key.startsWith('com-itemgather-') && key !== CACHE).map(key => caches.delete(key)))).then(() => self.clients.claim())) })
self.addEventListener('fetch', event => {
  const request = event.request
  if (request.method !== 'GET') return
  const url = new URL(request.url)
  if (url.origin !== location.origin || url.pathname.startsWith('/api/')) return
  if (request.mode === 'navigate') {
    event.respondWith(fetch(request).then(response => { const copy = response.clone(); caches.open(CACHE).then(cache => cache.put('/', copy)); return response }).catch(() => caches.match('/')))
    return
  }
  event.respondWith(caches.match(request).then(cached => cached || fetch(request).then(response => { if (response.ok && response.type === 'basic') { const copy = response.clone(); caches.open(CACHE).then(cache => cache.put(request, copy)) } return response })))
})
