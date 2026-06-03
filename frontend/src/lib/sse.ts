// SSE manager — connects to the BFF-proxied SSE endpoint.
// The BFF proxy injects the Clerk JWT server-side, so EventSource connects
// without Authorization header limitations.
// Backend SSE path: GET /api/v1/notifications/stream

import type { SSENotificationPayload } from './api-types'

export type SSEConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'failed'

export interface SSEHandlers {
  onNotification?:    (payload: SSENotificationPayload) => void
  onConnectionChange?:(status: SSEConnectionStatus) => void
}

const MAX_RETRIES = 10
// BFF SSE route — proxies /api/v1/notifications/stream with auth header injected server-side
const SSE_PATH = '/api/notifications/stream'

export function createSSEManager(handlers: SSEHandlers) {
  let es: EventSource | null = null
  let retries = 0
  let stopped = false

  function connect() {
    if (stopped) return

    handlers.onConnectionChange?.('connecting')
    es = new EventSource(SSE_PATH)

    es.onopen = () => {
      retries = 0
      handlers.onConnectionChange?.('connected')
    }

    // The backend emits `data: <json>\n\n` with no explicit `event:` field.
    // The default onmessage handler receives these unnamed events.
    es.onmessage = (e) => {
      try {
        const payload = JSON.parse(e.data) as SSENotificationPayload
        handlers.onNotification?.(payload)
      } catch {
        // ignore malformed event
      }
    }

    // Named `connected` event — server sends this on first connect
    es.addEventListener('connected', () => {
      handlers.onConnectionChange?.('connected')
    })

    // Named `heartbeat` event — no action needed
    es.addEventListener('heartbeat', () => {/* keep-alive */})

    es.onerror = () => {
      es?.close()
      es = null

      if (stopped) return

      if (retries >= MAX_RETRIES) {
        handlers.onConnectionChange?.('failed')
        return
      }

      handlers.onConnectionChange?.('reconnecting')
      const delay = Math.min(1000 * 2 ** retries, 30_000)
      retries++
      setTimeout(connect, delay)
    }
  }

  connect()

  return {
    disconnect() {
      stopped = true
      es?.close()
      es = null
    },
  }
}
