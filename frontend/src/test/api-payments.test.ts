import { describe, it, expect, vi, beforeEach } from "vitest";
import { api } from "@/lib/api";

// ── fetch mock ─────────────────────────────────────────────────────────────────

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

function okResponse(data: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(data),
  } as Response;
}

beforeEach(() => {
  mockFetch.mockReset();
  mockFetch.mockResolvedValue(okResponse({}));
});

// ── listMyPendingIntents ───────────────────────────────────────────────────────

describe("api.listMyPendingIntents", () => {
  it("calls /api/v1/payment-intents/my with auth header", async () => {
    mockFetch.mockResolvedValueOnce(okResponse([]));
    await api.listMyPendingIntents("tok123");
    const [url, init] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/payment-intents/my");
    expect((init as RequestInit & { headers: Record<string, string> }).headers["Authorization"]).toBe("Bearer tok123");
  });
});

// ── listMyIntents ─────────────────────────────────────────────────────────────

describe("api.listMyIntents", () => {
  it("calls /api/v1/payment-intents/my/all with auth header", async () => {
    mockFetch.mockResolvedValueOnce(okResponse([]));
    await api.listMyIntents("tok456");
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/payment-intents/my/all");
  });
});

// ── uploadComprobante ─────────────────────────────────────────────────────────

describe("api.uploadComprobante", () => {
  it("posts to the intent comprobante endpoint", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(null));
    const fd = new FormData();
    fd.append("file", new Blob(["data"]), "receipt.jpg");
    await api.uploadComprobante("tok", "token-abc", fd);
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/payment-intents/token-abc/comprobante");
  });

  it("encodes special characters in the intent token", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(null));
    await api.uploadComprobante("tok", "tok en/special", new FormData());
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain(encodeURIComponent("tok en/special"));
  });
});

// ── resubmitForReview ─────────────────────────────────────────────────────────

describe("api.resubmitForReview", () => {
  it("posts to the resubmit endpoint", async () => {
    mockFetch.mockResolvedValueOnce(okResponse({ token: "t", status: "under_review" }));
    const fd = new FormData();
    fd.append("notes", "Please reconsider");
    await api.resubmitForReview("tok", "token-xyz", fd);
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/payment-intents/token-xyz/resubmit");
  });
});

// ── adminRequestComprobante ───────────────────────────────────────────────────

describe("api.adminRequestComprobante", () => {
  it("posts to the admin request-comprobante endpoint", async () => {
    mockFetch.mockResolvedValueOnce(okResponse({ id: 1 }));
    await api.adminRequestComprobante("admintok", 42);
    const [url, init] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/admin/payment-intents/42/request-comprobante");
    expect((init as RequestInit).method).toBe("POST");
  });
});

// ── adminListPaymentIntents (URLSearchParams conditions) ──────────────────────

describe("api.adminListPaymentIntents", () => {
  const payload = { data: [], total: 0, page: { page: 1, limit: 15, total: 0 } };

  it("calls the endpoint with no query params when params is omitted", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/admin/payment-intents");
    expect(url).not.toContain("provider=");
    expect(url).not.toContain("status=");
  });

  it("appends provider when provided", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok", { provider: "paypal" });
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("provider=paypal");
  });

  it("appends status when provided", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok", { status: "pending" });
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("status=pending");
  });

  it("appends page when provided", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok", { page: 3 });
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("page=3");
  });

  it("appends limit when provided", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok", { limit: 25 });
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("limit=25");
  });

  it("appends all params when all provided", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok", {
      provider: "recurrente",
      status: "captured",
      page: 2,
      limit: 10,
    });
    const [url] = mockFetch.mock.calls[0];
    expect(url).toContain("provider=recurrente");
    expect(url).toContain("status=captured");
    expect(url).toContain("page=2");
    expect(url).toContain("limit=10");
  });

  it("passes empty-string provider and status when falsy (no key appended)", async () => {
    mockFetch.mockResolvedValueOnce(okResponse(payload));
    await api.adminListPaymentIntents("tok", { provider: "", status: "" });
    const [url] = mockFetch.mock.calls[0];
    expect(url).not.toContain("provider=");
    expect(url).not.toContain("status=");
  });
});

// ── adminCreditPaymentIntent ──────────────────────────────────────────────────

describe("api.adminCreditPaymentIntent", () => {
  it("posts with notes when provided", async () => {
    mockFetch.mockResolvedValueOnce(okResponse({ id: 5 }));
    await api.adminCreditPaymentIntent("tok", 5, "Manual credit approved");
    const [url, init] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/admin/payment-intents/5/credit");
    expect((init as RequestInit).method).toBe("POST");
    expect((init as RequestInit).body).toContain("Manual credit approved");
  });

  it("posts with empty notes when notes is omitted", async () => {
    mockFetch.mockResolvedValueOnce(okResponse({ id: 6 }));
    await api.adminCreditPaymentIntent("tok", 6);
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).body).toContain('""');
  });
});

// ── adminRejectPaymentIntent ──────────────────────────────────────────────────

describe("api.adminRejectPaymentIntent", () => {
  it("posts with rejection notes", async () => {
    mockFetch.mockResolvedValueOnce(okResponse({ id: 7 }));
    await api.adminRejectPaymentIntent("tok", 7, "Invalid comprobante");
    const [url, init] = mockFetch.mock.calls[0];
    expect(url).toContain("/api/v1/admin/payment-intents/7/reject");
    expect((init as RequestInit).body).toContain("Invalid comprobante");
  });
});

// ── adminPaymentIntentComprobanteUrl ──────────────────────────────────────────

describe("api.adminPaymentIntentComprobanteUrl", () => {
  it("returns the correct download URL string", () => {
    const url = api.adminPaymentIntentComprobanteUrl(99);
    expect(url).toBe("/api/v1/admin/payment-intents/99/comprobante");
  });
});
