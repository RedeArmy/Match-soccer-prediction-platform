import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  formatDate, formatDateTime, formatRelative, formatCountdown,
  truncate, initials, cn,
} from '@/lib/utils'

describe('cn (class name merger)', () => {
  it('merges class names', () => {
    expect(cn('foo', 'bar')).toBe('foo bar')
  })

  it('removes conflicting tailwind classes (last wins)', () => {
    expect(cn('text-red-400', 'text-blue-400')).toBe('text-blue-400')
  })

  it('handles conditional false values', () => {
    expect(cn('foo', false && 'bar')).toBe('foo')
  })
})

describe('formatDate', () => {
  it('returns a non-empty string for a valid ISO date', () => {
    const result = formatDate('2026-06-01T10:00:00Z')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })

  it('includes the year', () => {
    const result = formatDate('2026-06-01T10:00:00Z')
    expect(result).toContain('2026')
  })
})

describe('formatDateTime', () => {
  it('returns a longer string than formatDate (includes time)', () => {
    const date = formatDate('2026-06-01T10:00:00Z')
    const dateTime = formatDateTime('2026-06-01T10:00:00Z')
    expect(dateTime.length).toBeGreaterThan(date.length)
  })
})

describe('formatRelative', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('returns "ahora" or similar for a time a few seconds ago', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:10Z'))
    const result = formatRelative('2026-06-01T10:00:00Z')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })

  it('returns a relative string for a future date within 1h', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:00Z'))
    const result = formatRelative('2026-06-01T10:30:00Z')
    expect(result).toMatch(/min/)
  })

  it('falls back to formatted date for dates > 7 days away', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:00Z'))
    const result = formatRelative('2026-05-01T10:00:00Z')
    expect(result).toContain('2026')
  })
})

describe('formatCountdown', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('returns Finalizado for past date', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:00Z'))
    expect(formatCountdown('2026-05-31T10:00:00Z')).toBe('Finalizado')
  })

  it('shows days and hours for multi-day future', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:00Z'))
    const result = formatCountdown('2026-06-04T10:00:00Z')
    expect(result).toMatch(/3d/)
  })

  it('shows hours and minutes when less than a day', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:00Z'))
    const result = formatCountdown('2026-06-01T16:30:00Z')
    expect(result).toMatch(/6h/)
  })

  it('shows minutes only when less than an hour', () => {
    vi.setSystemTime(new Date('2026-06-01T10:00:00Z'))
    const result = formatCountdown('2026-06-01T10:45:00Z')
    expect(result).toMatch(/45m/)
  })
})

describe('truncate', () => {
  it('returns original string when shorter than maxLen', () => {
    expect(truncate('hello', 10)).toBe('hello')
  })

  it('truncates and appends ellipsis when longer than maxLen', () => {
    expect(truncate('hello world', 5)).toBe('hello…')
  })

  it('returns exact string when length equals maxLen', () => {
    expect(truncate('hello', 5)).toBe('hello')
  })
})

describe('initials', () => {
  it('extracts first letter of each word (max 2)', () => {
    expect(initials('Carlos López')).toBe('CL')
  })

  it('handles single name', () => {
    expect(initials('Carlos')).toBe('C')
  })

  it('ignores words beyond the second', () => {
    expect(initials('Juan Carlos López García')).toBe('JC')
  })

  it('uppercases letters', () => {
    expect(initials('ana beatriz')).toBe('AB')
  })
})
