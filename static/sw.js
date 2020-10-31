const VERSION = 3
const staticCacheName = `static-v${VERSION}`
const staticUrlsToCache = [
    'https://unpkg.com/@nicolasparada/router@0.8.0/router.js',
    'https://unpkg.com/focus-visible@5.2.0/dist/focus-visible.min.js',
    '/pages/access-page.js',
    '/pages/callback-page.js',
    '/pages/conversation-page.js',
    '/pages/home-page.js',
    '/pages/not-found-page.js',
    '/auth.js',
    '/http.js',
    '/index.html',
    '/main.js',
    '/shared.js',
    '/styles.css',
]

const cacheWhitelist = [
    staticCacheName,
]

self.addEventListener('install', ev => {
    ev.waitUntil(
        caches.open(staticCacheName).then(cache => cache.addAll(staticUrlsToCache))
    )
})

self.addEventListener('activate', ev => {
    ev.waitUntil(
        caches.keys().then(cacheNames => Promise.all(cacheNames
            .filter(cacheName => !cacheWhitelist.includes(cacheName))
            .map(cacheName => caches.delete(cacheName))
        ))
    )
})

self.addEventListener('fetch', ev => {
    ev.respondWith(
        caches.match(ev.request).then(res => res || fetch(ev.request).catch(err => {
            if (ev.request.mode === 'navigate') {
                return caches.match('/index.html')
            }
            throw err
        }))
    )
})
