// Private channel for session-expiry notifications.
//
// Using a module-level EventTarget (not globalThis) means only code that
// imports this module can dispatch or subscribe to the event. External scripts,
// browser extensions, and injected iframes cannot trigger a forced sign-out by
// dispatching a synthetic event on window.
const bus = new EventTarget();

const SESSION_EXPIRED = "session-expired";

export function dispatchSessionExpired(): void {
  bus.dispatchEvent(new Event(SESSION_EXPIRED));
}

export function onSessionExpired(handler: () => void): () => void {
  bus.addEventListener(SESSION_EXPIRED, handler);
  return () => bus.removeEventListener(SESSION_EXPIRED, handler);
}
