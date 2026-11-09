'use strict';

// A service worker that does nothing. The experiment only needs to know
// whether the browser accepts one on this connection.
self.addEventListener('install', function () {
  self.skipWaiting();
});

self.addEventListener('activate', function (event) {
  event.waitUntil(self.clients.claim());
});
