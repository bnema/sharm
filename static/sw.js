// Minimal service worker for PWA installability
self.addEventListener('fetch', (event) => {
  event.respondWith(fetch(event.request));
});
