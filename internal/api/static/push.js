/**
 * push.js — Web Push subscription manager
 *
 * Provides two exported functions consumed by the application shell:
 *
 *   registerPushSubscription(authToken)   → PushSubscription | null
 *   unregisterPushSubscription(authToken) → void
 *
 * Usage (ES module import):
 *
 *   import { registerPushSubscription } from '/push.js';
 *   const sub = await registerPushSubscription(sessionToken);
 *
 * The module handles feature detection, permission prompts, VAPID key
 * fetching, and subscription POSTing to the API.  It is safe to call
 * registerPushSubscription on every page load — if a valid subscription
 * already exists it is reused rather than re-created.
 */

'use strict';

const SW_PATH            = '/sw.js';
const VAPID_KEY_ENDPOINT = '/api/v1/push/vapid-public-key';
const SUBSCRIBE_ENDPOINT = '/api/v1/push/subscribe';

// ── Public API ────────────────────────────────────────────────────────────────

/**
 * Register or refresh a Web Push subscription for the authenticated user.
 *
 * @param {string} authToken  Bearer token for the authenticated session.
 * @returns {Promise<PushSubscription|null>}  The active subscription, or null
 *   if the browser does not support push, the user denied permission, or a
 *   network error occurred.
 */
async function registerPushSubscription(authToken) {
  if (!isPushSupported()) {
    console.warn('[push.js] Web Push is not supported in this browser');
    return null;
  }

  const permission = await Notification.requestPermission();
  if (permission !== 'granted') {
    console.info('[push.js] Notification permission not granted:', permission);
    return null;
  }

  try {
    const registration = await navigator.serviceWorker.register(SW_PATH, { scope: '/' });
    await navigator.serviceWorker.ready;

    const vapidPublicKey = await fetchVAPIDPublicKey(authToken);
    const applicationServerKey = urlBase64ToUint8Array(vapidPublicKey);

    // Reuse an existing subscription to avoid registering a duplicate endpoint.
    let subscription = await registration.pushManager.getSubscription();
    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly:      true,
        applicationServerKey,
      });
    }

    await postSubscription(subscription, authToken);
    return subscription;
  } catch (err) {
    console.error('[push.js] Failed to register push subscription:', err);
    return null;
  }
}

/**
 * Unsubscribe the current push subscription and notify the server.
 *
 * @param {string} authToken  Bearer token for the authenticated session.
 * @returns {Promise<void>}
 */
async function unregisterPushSubscription(authToken) {
  if (!isPushSupported()) return;

  const registration = await navigator.serviceWorker.getRegistration(SW_PATH);
  if (!registration) return;

  const subscription = await registration.pushManager.getSubscription();
  if (!subscription) return;

  try {
    await fetch(SUBSCRIBE_ENDPOINT, {
      method:  'DELETE',
      headers: {
        'Content-Type':  'application/json',
        'Authorization': `Bearer ${authToken}`,
      },
      body: JSON.stringify({ endpoint: subscription.endpoint }),
    });
  } catch (err) {
    console.warn('[push.js] Failed to notify server of unsubscription:', err);
  }

  await subscription.unsubscribe();
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function isPushSupported() {
  return 'serviceWorker' in navigator && 'PushManager' in globalThis && 'Notification' in globalThis;
}

async function fetchVAPIDPublicKey(authToken) {
  const res = await fetch(VAPID_KEY_ENDPOINT, {
    headers: { 'Authorization': `Bearer ${authToken}` },
  });
  if (!res.ok) {
    throw new Error(`Failed to fetch VAPID public key: HTTP ${res.status}`);
  }
  const { vapid_public_key } = await res.json();
  if (!vapid_public_key) {
    throw new Error('Server returned empty VAPID public key');
  }
  return vapid_public_key;
}

async function postSubscription(subscription, authToken) {
  const keys = {
    p256dh_key: arrayBufferToBase64Url(subscription.getKey('p256dh')),
    auth_key:   arrayBufferToBase64Url(subscription.getKey('auth')),
  };

  const res = await fetch(SUBSCRIBE_ENDPOINT, {
    method:  'POST',
    headers: {
      'Content-Type':  'application/json',
      'Authorization': `Bearer ${authToken}`,
    },
    body: JSON.stringify({
      endpoint:   subscription.endpoint,
      p256dh_key: keys.p256dh_key,
      auth_key:   keys.auth_key,
      user_agent: navigator.userAgent,
    }),
  });

  if (!res.ok) {
    throw new Error(`Failed to register subscription with server: HTTP ${res.status}`);
  }
}

/**
 * Convert a Base64URL string (as returned by the server's VAPID endpoint) to
 * the Uint8Array required by PushManager.subscribe().
 */
function urlBase64ToUint8Array(base64String) {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64  = (base64String + padding).replaceAll('-', '+').replaceAll('_', '/');
  const rawData = atob(base64);
  return Uint8Array.from(rawData, (c) => c.charCodeAt(0));
}

/**
 * Encode an ArrayBuffer returned by PushSubscription.getKey() as Base64URL
 * without padding, matching the format expected by the server's p256dh_key and
 * auth_key fields.
 */
function arrayBufferToBase64Url(buffer) {
  return btoa(String.fromCharCode(...new Uint8Array(buffer)))
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replace(/={1,2}$/, '');
}

export { registerPushSubscription, unregisterPushSubscription };
