import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// ── Currency formatters ───────────────────────────────────────────────────────

export function formatGTQ(cents: number): string {
  return new Intl.NumberFormat('es-GT', {
    style: 'currency',
    currency: 'GTQ',
    minimumFractionDigits: 2,
  }).format(cents / 100)
}

export function formatUSD(cents: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
  }).format(cents / 100)
}

export function centsToGTQ(cents: number): number {
  return cents / 100
}

// Convert GTQ amount to USD using sell rate (user pays sell rate to deposit USD)
export function gtqToUSD(gtqAmount: number, sellRate: string): number {
  const rate = Number.parseFloat(sellRate)
  if (!rate || rate === 0) return 0
  return gtqAmount / rate
}

// Convert USD amount to GTQ using buy rate (we buy USD at buy rate)
export function usdToGTQ(usdAmount: number, buyRate: string): number {
  const rate = parseFloat(buyRate)
  if (!rate) return 0
  return usdAmount * rate
}

// ── Date formatters ───────────────────────────────────────────────────────────

export function formatDate(iso: string): string {
  return new Intl.DateTimeFormat('es-GT', {
    year:  'numeric',
    month: 'short',
    day:   'numeric',
  }).format(new Date(iso))
}

export function formatDateTime(iso: string): string {
  return new Intl.DateTimeFormat('es-GT', {
    year:   'numeric',
    month:  'short',
    day:    'numeric',
    hour:   '2-digit',
    minute: '2-digit',
  }).format(new Date(iso))
}

export function formatRelative(iso: string): string {
  const rtf = new Intl.RelativeTimeFormat('es', { numeric: 'auto' })
  const diff = (new Date(iso).getTime() - Date.now()) / 1000

  if (Math.abs(diff) < 60)     return rtf.format(Math.round(diff), 'second')
  if (Math.abs(diff) < 3600)   return rtf.format(Math.round(diff / 60), 'minute')
  if (Math.abs(diff) < 86400)  return rtf.format(Math.round(diff / 3600), 'hour')
  if (Math.abs(diff) < 604800) return rtf.format(Math.round(diff / 86400), 'day')
  return formatDate(iso)
}

export function formatCountdown(iso: string): string {
  const diff = new Date(iso).getTime() - Date.now()
  if (diff <= 0) return 'Finalizado'

  const d = Math.floor(diff / 86_400_000)
  const h = Math.floor((diff % 86_400_000) / 3_600_000)
  const m = Math.floor((diff % 3_600_000) / 60_000)

  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

// ── Exchange rate formatters ──────────────────────────────────────────────────

export function formatRate(rate: string): string {
  const n = Number.parseFloat(rate)
  return Number.isNaN(n) ? rate : n.toFixed(4)
}

// ── General ───────────────────────────────────────────────────────────────────

export function truncate(str: string, maxLen: number): string {
  return str.length > maxLen ? `${str.slice(0, maxLen)}…` : str
}

export function initials(name: string): string {
  return name
    .split(' ')
    .slice(0, 2)
    .map(w => w[0]?.toUpperCase() ?? '')
    .join('')
}

// Sniff MIME type from the first bytes of a File (client-side validation)
export async function sniffMIME(file: File): Promise<string> {
  const buf = await file.slice(0, 512).arrayBuffer()
  const bytes = new Uint8Array(buf)

  // PDF: %PDF
  if (bytes[0] === 0x25 && bytes[1] === 0x50 && bytes[2] === 0x44 && bytes[3] === 0x46) {
    return 'application/pdf'
  }
  // JPEG: FF D8 FF
  if (bytes[0] === 0xFF && bytes[1] === 0xD8 && bytes[2] === 0xFF) {
    return 'image/jpeg'
  }
  // PNG: 89 50 4E 47
  if (bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4E && bytes[3] === 0x47) {
    return 'image/png'
  }
  // WebP: RIFF....WEBP
  if (bytes[0] === 0x52 && bytes[1] === 0x49 && bytes[2] === 0x46 && bytes[3] === 0x46 &&
      bytes[8] === 0x57 && bytes[9] === 0x45 && bytes[10] === 0x42 && bytes[11] === 0x50) {
    return 'image/webp'
  }
  return file.type || 'application/octet-stream'
}

export const ALLOWED_UPLOAD_TYPES = ['image/jpeg', 'image/png', 'image/webp', 'application/pdf'] as const
export type AllowedUploadType = typeof ALLOWED_UPLOAD_TYPES[number]

export function isAllowedUploadType(mime: string): mime is AllowedUploadType {
  return (ALLOWED_UPLOAD_TYPES as readonly string[]).includes(mime)
}
