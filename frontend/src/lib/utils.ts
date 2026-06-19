import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// ── Currency formatters ───────────────────────────────────────────────────────

export function formatGTQ(cents: number): string {
  return new Intl.NumberFormat("es-GT", {
    style: "currency",
    currency: "GTQ",
    minimumFractionDigits: 2,
  }).format(cents / 100);
}

export function formatUSD(cents: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(cents / 100);
}

export function centsToGTQ(cents: number): number {
  return cents / 100;
}

// Convert GTQ amount to USD using sell rate (user pays sell rate to deposit USD)
export function gtqToUSD(gtqAmount: number, sellRate: string): number {
  const rate = Number.parseFloat(sellRate);
  if (!rate || rate === 0) return 0;
  return gtqAmount / rate;
}

// Convert USD amount to GTQ using buy rate (we buy USD at buy rate)
export function usdToGTQ(usdAmount: number, buyRate: string): number {
  const rate = Number.parseFloat(buyRate);
  if (!rate) return 0;
  return usdAmount * rate;
}

// ── Date formatters ───────────────────────────────────────────────────────────

function parseDate(iso: string | null | undefined): Date | null {
  if (!iso) return null;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function formatDate(iso: string | null | undefined): string {
  const d = parseDate(iso);
  if (!d) return "—";
  return new Intl.DateTimeFormat("es-GT", {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(d);
}

export function formatDateTime(iso: string | null | undefined): string {
  const d = parseDate(iso);
  if (!d) return "—";
  return new Intl.DateTimeFormat("es-GT", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
}

export function formatRelative(
  iso: string | null | undefined,
  nowMs = Date.now(),
): string {
  const d = parseDate(iso);
  if (!d) return "—";
  const rtf = new Intl.RelativeTimeFormat("es", { numeric: "auto" });
  const diff = (d.getTime() - nowMs) / 1000;

  if (Math.abs(diff) < 60) return rtf.format(Math.round(diff), "second");
  if (Math.abs(diff) < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (Math.abs(diff) < 86400)
    return rtf.format(Math.round(diff / 3600), "hour");
  if (Math.abs(diff) < 604800)
    return rtf.format(Math.round(diff / 86400), "day");
  return formatDate(iso);
}

export function formatCountdown(
  iso: string | null | undefined,
  nowMs = Date.now(),
): string {
  const d = parseDate(iso);
  if (!d) return "—";
  const diff = d.getTime() - nowMs;
  if (diff <= 0) return "Finalizado";

  const days = Math.floor(diff / 86_400_000);
  const hours = Math.floor((diff % 86_400_000) / 3_600_000);
  const mins = Math.floor((diff % 3_600_000) / 60_000);

  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

// ── Exchange rate formatters ──────────────────────────────────────────────────

export function formatRate(rate: string): string {
  const n = Number.parseFloat(rate);
  return Number.isNaN(n) ? rate : n.toFixed(4);
}

// ── Ledger kind → i18n key ────────────────────────────────────────────────────

// Internal reservation entries should not appear in user-facing history.
// withdrawal_reserve only changes reserved_cents, not actual balance.
export function isVisibleLedgerKind(kind: string): boolean {
  return kind !== "withdrawal_reserve";
}

export function ledgerKindKey(kind: string): string {
  if (kind === "webhook_recurrente") return "ledger.creditRecurrente";
  if (kind === "webhook_paypal") return "ledger.creditPayPal";
  if (kind === "bank_transfer") return "ledger.creditBank";
  if (kind === "withdrawal_release") return "ledger.refund";
  if (kind.startsWith("withdrawal_")) return "ledger.withdrawal";
  if (kind === "prize") return "ledger.earning";
  if (kind === "entry_fee") return "ledger.entryFee";
  return kind;
}

// ── General ───────────────────────────────────────────────────────────────────

export function truncate(str: string, maxLen: number): string {
  return str.length > maxLen ? `${str.slice(0, maxLen)}…` : str;
}

export function initials(name: string): string {
  return name
    .split(" ")
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("");
}

// Sniff MIME type from the first bytes of a File (client-side validation)
export async function sniffMIME(file: File): Promise<string> {
  const buf = await file.slice(0, 512).arrayBuffer();
  const bytes = new Uint8Array(buf);

  // PDF: %PDF
  if (
    bytes[0] === 0x25 &&
    bytes[1] === 0x50 &&
    bytes[2] === 0x44 &&
    bytes[3] === 0x46
  ) {
    return "application/pdf";
  }
  // JPEG: FF D8 FF
  if (bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) {
    return "image/jpeg";
  }
  // PNG: 89 50 4E 47
  if (
    bytes[0] === 0x89 &&
    bytes[1] === 0x50 &&
    bytes[2] === 0x4e &&
    bytes[3] === 0x47
  ) {
    return "image/png";
  }
  // WebP: RIFF....WEBP
  if (
    bytes[0] === 0x52 &&
    bytes[1] === 0x49 &&
    bytes[2] === 0x46 &&
    bytes[3] === 0x46 &&
    bytes[8] === 0x57 &&
    bytes[9] === 0x45 &&
    bytes[10] === 0x42 &&
    bytes[11] === 0x50
  ) {
    return "image/webp";
  }
  return file.type || "application/octet-stream";
}

export const ALLOWED_UPLOAD_TYPES = [
  "image/jpeg",
  "image/png",
  "image/webp",
  "application/pdf",
] as const;
export type AllowedUploadType = (typeof ALLOWED_UPLOAD_TYPES)[number];

export function isAllowedUploadType(mime: string): mime is AllowedUploadType {
  return (ALLOWED_UPLOAD_TYPES as readonly string[]).includes(mime);
}

export function formatAmount(cents: number, currency: string): string {
  if (currency === "USD") return `$${(cents / 100).toFixed(2)} USD`;
  return `Q${(cents / 100).toFixed(2)}`;
}
