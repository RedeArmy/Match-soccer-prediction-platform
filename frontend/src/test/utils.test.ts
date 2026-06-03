import { describe, it, expect } from 'vitest'
import {
  formatGTQ, formatUSD, formatCountdown, formatRate,
  sniffMIME, isAllowedUploadType, usdToGTQ, gtqToUSD,
} from '@/lib/utils'

describe('formatGTQ', () => {
  it('formats cents as GTQ currency', () => {
    expect(formatGTQ(100_00)).toContain('100')
    expect(formatGTQ(100_00)).toContain('Q')
  })

  it('handles zero', () => {
    expect(formatGTQ(0)).toContain('0')
  })
})

describe('formatUSD', () => {
  it('formats cents as USD currency', () => {
    expect(formatUSD(50_00)).toContain('50')
    expect(formatUSD(50_00)).toContain('$')
  })
})

describe('formatCountdown', () => {
  it('returns Finalizado for past dates', () => {
    const past = new Date(Date.now() - 1000).toISOString()
    expect(formatCountdown(past)).toBe('Finalizado')
  })

  it('returns days remaining for future dates', () => {
    const future = new Date(Date.now() + 2 * 86_400_000).toISOString()
    expect(formatCountdown(future)).toContain('d')
  })
})

describe('formatRate', () => {
  it('formats to 4 decimal places', () => {
    expect(formatRate('7.8')).toBe('7.8000')
  })

  it('returns original string for invalid input', () => {
    expect(formatRate('invalid')).toBe('invalid')
  })
})

describe('currency conversion', () => {
  it('converts USD to GTQ', () => {
    const result = usdToGTQ(100, '7.80')
    expect(result).toBeCloseTo(780, 0)
  })

  it('converts GTQ to USD', () => {
    const result = gtqToUSD(780, '7.80')
    expect(result).toBeCloseTo(100, 0)
  })
})

describe('sniffMIME', () => {
  it('detects JPEG from header bytes', async () => {
    const bytes = new Uint8Array([0xFF, 0xD8, 0xFF, 0xE0, ...new Array(508).fill(0)])
    const file = new File([bytes], 'test.jpg')
    const mime = await sniffMIME(file)
    expect(mime).toBe('image/jpeg')
  })

  it('detects PNG from header bytes', async () => {
    const bytes = new Uint8Array([0x89, 0x50, 0x4E, 0x47, ...new Array(508).fill(0)])
    const file = new File([bytes], 'test.png')
    const mime = await sniffMIME(file)
    expect(mime).toBe('image/png')
  })

  it('detects PDF from header bytes', async () => {
    const bytes = new Uint8Array([0x25, 0x50, 0x44, 0x46, ...new Array(508).fill(0)])
    const file = new File([bytes], 'test.pdf')
    const mime = await sniffMIME(file)
    expect(mime).toBe('application/pdf')
  })
})

describe('isAllowedUploadType', () => {
  it('accepts allowed MIME types', () => {
    expect(isAllowedUploadType('image/jpeg')).toBe(true)
    expect(isAllowedUploadType('image/png')).toBe(true)
    expect(isAllowedUploadType('image/webp')).toBe(true)
    expect(isAllowedUploadType('application/pdf')).toBe(true)
  })

  it('rejects disallowed MIME types', () => {
    expect(isAllowedUploadType('application/zip')).toBe(false)
    expect(isAllowedUploadType('text/plain')).toBe(false)
    expect(isAllowedUploadType('image/gif')).toBe(false)
  })
})
