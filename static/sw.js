// Minimal service worker for PWA installability
// @ts-nocheck - Service worker types not included

self.addEventListener('fetch', (event) => {
  event.respondWith(fetch(event.request));
});
