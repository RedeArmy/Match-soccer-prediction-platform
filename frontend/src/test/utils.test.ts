import { describe, it, expect } from "vitest";
import {
  formatGTQ,
  formatUSD,
  formatCountdown,
  formatRate,
  sniffMIME,
  isAllowedUploadType,
  usdToGTQ,
  gtqToUSD,
  centsToGTQ,
  isVisibleLedgerKind,
  ledgerKindKey,
  truncate,
  initials,
} from "@/lib/utils";

describe("formatGTQ", () => {
  it("formats cents as GTQ currency", () => {
    expect(formatGTQ(100_00)).toContain("100");
    expect(formatGTQ(100_00)).toContain("Q");
  });

  it("handles zero", () => {
    expect(formatGTQ(0)).toContain("0");
  });
});

describe("formatUSD", () => {
  it("formats cents as USD currency", () => {
    expect(formatUSD(50_00)).toContain("50");
    expect(formatUSD(50_00)).toContain("$");
  });
});

describe("formatCountdown", () => {
  it("returns Finalizado for past dates", () => {
    const past = new Date(Date.now() - 1000).toISOString();
    expect(formatCountdown(past)).toBe("Finalizado");
  });

  it("returns days remaining for future dates", () => {
    const future = new Date(Date.now() + 2 * 86_400_000).toISOString();
    expect(formatCountdown(future)).toContain("d");
  });
});

describe("formatRate", () => {
  it("formats to 4 decimal places", () => {
    expect(formatRate("7.8")).toBe("7.8000");
  });

  it("returns original string for invalid input", () => {
    expect(formatRate("invalid")).toBe("invalid");
  });
});

describe("centsToGTQ", () => {
  it("divides cents by 100", () => {
    expect(centsToGTQ(100)).toBe(1);
    expect(centsToGTQ(780)).toBeCloseTo(7.8, 1);
    expect(centsToGTQ(0)).toBe(0);
  });
});

describe("currency conversion", () => {
  it("converts USD to GTQ", () => {
    const result = usdToGTQ(100, "7.80");
    expect(result).toBeCloseTo(780, 0);
  });

  it("converts GTQ to USD", () => {
    const result = gtqToUSD(780, "7.80");
    expect(result).toBeCloseTo(100, 0);
  });

  it("usdToGTQ returns 0 for invalid rate", () => {
    expect(usdToGTQ(100, "0")).toBe(0);
    expect(usdToGTQ(100, "invalid")).toBe(0);
  });

  it("gtqToUSD returns 0 for invalid rate", () => {
    expect(gtqToUSD(780, "0")).toBe(0);
    expect(gtqToUSD(780, "invalid")).toBe(0);
  });
});

describe("sniffMIME", () => {
  it("detects JPEG from header bytes", async () => {
    const bytes = new Uint8Array([
      0xff,
      0xd8,
      0xff,
      0xe0,
      ...new Array(508).fill(0),
    ]);
    const file = new File([bytes], "test.jpg");
    const mime = await sniffMIME(file);
    expect(mime).toBe("image/jpeg");
  });

  it("detects PNG from header bytes", async () => {
    const bytes = new Uint8Array([
      0x89,
      0x50,
      0x4e,
      0x47,
      ...new Array(508).fill(0),
    ]);
    const file = new File([bytes], "test.png");
    const mime = await sniffMIME(file);
    expect(mime).toBe("image/png");
  });

  it("detects PDF from header bytes", async () => {
    const bytes = new Uint8Array([
      0x25,
      0x50,
      0x44,
      0x46,
      ...new Array(508).fill(0),
    ]);
    const file = new File([bytes], "test.pdf");
    const mime = await sniffMIME(file);
    expect(mime).toBe("application/pdf");
  });

  it("detects WebP from header bytes", async () => {
    const bytes = new Uint8Array(512).fill(0);
    bytes[0] = 0x52;
    bytes[1] = 0x49;
    bytes[2] = 0x46;
    bytes[3] = 0x46;
    bytes[8] = 0x57;
    bytes[9] = 0x45;
    bytes[10] = 0x42;
    bytes[11] = 0x50;
    const file = new File([bytes], "test.webp");
    expect(await sniffMIME(file)).toBe("image/webp");
  });

  it("falls back to file.type when bytes are unknown and type is set", async () => {
    const bytes = new Uint8Array([
      0x00,
      0x01,
      0x02,
      0x03,
      ...new Array(508).fill(0),
    ]);
    const file = new File([bytes], "test.txt", { type: "text/plain" });
    expect(await sniffMIME(file)).toBe("text/plain");
  });

  it("falls back to application/octet-stream when bytes unknown and no type", async () => {
    const bytes = new Uint8Array([
      0x00,
      0x01,
      0x02,
      0x03,
      ...new Array(508).fill(0),
    ]);
    const file = new File([bytes], "test.bin");
    expect(await sniffMIME(file)).toBe("application/octet-stream");
  });
});

describe("isAllowedUploadType", () => {
  it("accepts allowed MIME types", () => {
    expect(isAllowedUploadType("image/jpeg")).toBe(true);
    expect(isAllowedUploadType("image/png")).toBe(true);
    expect(isAllowedUploadType("image/webp")).toBe(true);
    expect(isAllowedUploadType("application/pdf")).toBe(true);
  });

  it("rejects disallowed MIME types", () => {
    expect(isAllowedUploadType("application/zip")).toBe(false);
    expect(isAllowedUploadType("text/plain")).toBe(false);
    expect(isAllowedUploadType("image/gif")).toBe(false);
  });
});

// ── isVisibleLedgerKind ───────────────────────────────────────────────────────

describe("isVisibleLedgerKind", () => {
  it("returns false for withdrawal_reserve", () => {
    expect(isVisibleLedgerKind("withdrawal_reserve")).toBe(false);
  });

  it("returns true for all other kinds", () => {
    expect(isVisibleLedgerKind("webhook_recurrente")).toBe(true);
    expect(isVisibleLedgerKind("webhook_paypal")).toBe(true);
    expect(isVisibleLedgerKind("prize")).toBe(true);
    expect(isVisibleLedgerKind("entry_fee")).toBe(true);
    expect(isVisibleLedgerKind("withdrawal_debit")).toBe(true);
  });
});

// ── ledgerKindKey ─────────────────────────────────────────────────────────────

describe("ledgerKindKey", () => {
  it("maps webhook_recurrente to ledger.creditRecurrente", () => {
    expect(ledgerKindKey("webhook_recurrente")).toBe("ledger.creditRecurrente");
  });

  it("maps webhook_paypal to ledger.creditPayPal", () => {
    expect(ledgerKindKey("webhook_paypal")).toBe("ledger.creditPayPal");
  });

  it("maps bank_transfer to ledger.creditBank", () => {
    expect(ledgerKindKey("bank_transfer")).toBe("ledger.creditBank");
  });

  it("maps withdrawal_release to ledger.refund", () => {
    expect(ledgerKindKey("withdrawal_release")).toBe("ledger.refund");
  });

  it("maps withdrawal_debit to ledger.withdrawal", () => {
    expect(ledgerKindKey("withdrawal_debit")).toBe("ledger.withdrawal");
  });

  it("maps prize to ledger.earning", () => {
    expect(ledgerKindKey("prize")).toBe("ledger.earning");
  });

  it("maps entry_fee to ledger.entryFee", () => {
    expect(ledgerKindKey("entry_fee")).toBe("ledger.entryFee");
  });

  it("returns raw kind for unknown values", () => {
    expect(ledgerKindKey("custom_kind")).toBe("custom_kind");
  });
});

// ── truncate ──────────────────────────────────────────────────────────────────

describe("truncate", () => {
  it("returns string unchanged when shorter than maxLen", () => {
    expect(truncate("hello", 10)).toBe("hello");
  });

  it("truncates and appends ellipsis when longer than maxLen", () => {
    expect(truncate("hello world", 5)).toBe("hello…");
  });

  it("returns string unchanged when equal to maxLen", () => {
    expect(truncate("hello", 5)).toBe("hello");
  });
});

// ── initials ──────────────────────────────────────────────────────────────────

describe("initials", () => {
  it("returns two uppercase initials for a two-word name", () => {
    expect(initials("Juan Pérez")).toBe("JP");
  });

  it("returns single initial for a one-word name", () => {
    expect(initials("Ana")).toBe("A");
  });

  it("ignores words beyond the first two", () => {
    expect(initials("María Luisa García")).toBe("ML");
  });
});
