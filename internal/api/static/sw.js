/**
 * sw.js — World Cup Quiniela Service Worker
 *
 * Handles Web Push notifications delivered by the server even when the
 * application tab is closed.  The Service Worker is registered at the root
 * scope (/sw.js) so it controls all pages of the origin.
 *
 * Payload contract (must match the Go pushPayload struct in
 * internal/notification/dispatcher/user.go):
 *
 *   {
 *     "notification_id": 42,
 *     "type": "payment.confirmed",
 *     "title": "Pago confirmado",
 *     "body":  "Tu depósito de Q250 fue acreditado",
 *     "action_url": "/balance",
 *     "icon":  "/icons/icon-192.png",
 *     "badge": "/icons/badge-72.png"
 *   }
 */

'use strict';

// ── Push event ────────────────────────────────────────────────────────────────

self.addEventListener('push', (event) => {
  if (!event.data) return;

  let payload;
  try {
    payload = event.data.json();
  } catch {
    // Malformed payload — show a generic fallback so the push is not silently lost.
    payload = { title: 'Nueva notificación', body: '' };
  }

  const title = payload.title || 'World Cup Quiniela';
  const options = {
    body:    payload.body        || '',
    icon:    payload.icon        || '/icons/icon-192.png',
    badge:   payload.badge       || '/icons/badge-72.png',
    // tag deduplicates: a second push with the same tag replaces the first
    // notification rather than stacking, keeping the notification centre clean.
    tag:     `wcq-${payload.notification_id || Date.now()}`,
    renotify: false,
    data: {
      url:             payload.action_url    || '/',
      notification_id: payload.notification_id,
    },
  };

  event.waitUntil(self.registration.showNotification(title, options));
});

// ── Notification click ────────────────────────────────────────────────────────

self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const targetUrl = (event.notification.data && event.notification.data.url) || '/';

  event.waitUntil(
    clients
      .matchAll({ type: 'window', includeUncontrolled: true })
      .then((clientList) => {
        // Focus an existing tab that already shows the target path.
        for (const client of clientList) {
          if (new URL(client.url).pathname === new URL(targetUrl, self.location.origin).pathname) {
            return client.focus();
          }
        }
        // Otherwise open a new tab.
        return clients.openWindow(targetUrl);
      }),
  );
});

// ── Subscription change ───────────────────────────────────────────────────────
//
// Fired by the browser when an existing push subscription expires and the
// browser has automatically renewed it.  We must POST the new subscription
// to the server so the next push reaches the correct endpoint.

self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(resubscribe(event));
});

async function resubscribe(event) {
  const subscription = await self.registration.pushManager.subscribe(
    event.oldSubscription.options,
  );

  await fetch('/api/v1/push/subscribe', {
    method:  'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      endpoint:    subscription.endpoint,
      p256dh_key:  arrayBufferToBase64Url(subscription.getKey('p256dh')),
      auth_key:    arrayBufferToBase64Url(subscription.getKey('auth')),
      user_agent:  navigator.userAgent,
    }),
  });
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function arrayBufferToBase64Url(buffer) {
  return btoa(String.fromCharCode(...new Uint8Array(buffer)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/={1,2}$/, '');
}
