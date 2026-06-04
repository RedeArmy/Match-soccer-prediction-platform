import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Mock EventSource (not available in jsdom)
class MockEventSource {
  static OPEN = 1
  url: string
  onopen:   ((e: Event) => void) | null = null
  onmessage:((e: MessageEvent) => void) | null = null
  onerror:  ((e: Event) => void) | null = null
  private listeners: Map<string, ((e: MessageEvent) => void)[]> = new Map()

  constructor(url: string) { this.url = url }

  addEventListener(type: string, cb: (e: MessageEvent) => void) {
    const list = this.listeners.get(type) ?? []
    list.push(cb)
    this.listeners.set(type, list)
  }

  dispatchMessage(data: unknown) {
    const e = { data: JSON.stringify(data) } as MessageEvent
    this.onmessage?.(e)
  }

  dispatchNamed(type: string, data: unknown) {
    const e = { data: JSON.stringify(data) } as MessageEvent
    this.listeners.get(type)?.forEach(cb => cb(e))
  }

  dispatchError() {
    this.onerror?.({} as Event)
  }

  close() {/* noop */}
}

vi.stubGlobal('EventSource', MockEventSource)

import { createSSEManager } from '@/lib/sse'

describe('createSSEManager', () => {
  let es: MockEventSource

  beforeEach(() => {
    vi.useFakeTimers()
    es = new MockEventSource('/api/notifications/stream')
    vi.spyOn(globalThis, 'EventSource').mockImplementation(function MockedEventSource() {
      return es
    } as unknown as typeof EventSource)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('calls onConnectionChange with connecting on start', () => {
    const onConnectionChange = vi.fn()
    createSSEManager({ onConnectionChange })
    expect(onConnectionChange).toHaveBeenCalledWith('connecting')
  })

  it('calls onNotification when a message is received', () => {
    const onNotification = vi.fn()
    createSSEManager({ onNotification })

    const payload = { id: 1, event_type: 'score.updated', title: 'Test', body: '', action_url: null, read: false, created_at: '' }
    es.dispatchMessage(payload)

    expect(onNotification).toHaveBeenCalledWith(payload)
  })

  it('reconnects with backoff on error', () => {
    const onConnectionChange = vi.fn()
    createSSEManager({ onConnectionChange })

    es.dispatchError()
    expect(onConnectionChange).toHaveBeenCalledWith('reconnecting')

    // Advance past backoff delay (1000ms × 2^0 = 1s)
    vi.advanceTimersByTime(1100)
    expect(onConnectionChange).toHaveBeenCalledWith('connecting')
  })

  it('stops reconnecting after disconnect()', () => {
    const onConnectionChange = vi.fn()
    const { disconnect } = createSSEManager({ onConnectionChange })

    disconnect()
    onConnectionChange.mockClear()

    es.dispatchError()
    vi.advanceTimersByTime(2000)

    expect(onConnectionChange).not.toHaveBeenCalled()
  })
})
