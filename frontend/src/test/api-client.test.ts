import { describe, it, expect, vi, beforeEach } from "vitest";

// Hoist Clerk mock so it is available before module imports
vi.mock("@clerk/nextjs/server", () => ({ auth: vi.fn() }));

// ── Fetch mock ────────────────────────────────────────────────────────────────

function makeResponse(
  body: unknown,
  status = 200,
  headers: Record<string, string> = {},
): Response {
  const bodyStr = body === null ? null : JSON.stringify(body);
  return new Response(bodyStr, {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

// Import after mocks are in place
import { api, serverAPI } from "@/lib/api";
import { getServerToken } from "@/lib/auth";
import { auth } from "@clerk/nextjs/server";

// ── api (browser singleton) ───────────────────────────────────────────────────

describe("api – GET request without token", () => {
  beforeEach(() => mockFetch.mockReset());

  it("does not send Authorization header", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        buy_rate: "7.72",
        sell_rate: "7.80",
        effective_at: "",
        stale: false,
      }),
    );
    await api.getExchangeRate();
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBeUndefined();
  });
});

describe("api – GET request with token", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends Authorization: Bearer <token>", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ available_cents: 0, reserved_cents: 0, pending_cents: 0 }),
    );
    await api.getBalance("tok_test_123");
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok_test_123");
  });
});

describe("api – error response with parseable body", () => {
  beforeEach(() => mockFetch.mockReset());

  it("throws with .message, .code, .status", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse(
        { error: { message: "Not found", code: "ERR_NOT_FOUND" } },
        404,
      ),
    );
    const err = await api.getBalance("tok").catch((e) => e);
    expect(err.message).toBe("Not found");
    expect(err.code).toBe("ERR_NOT_FOUND");
    expect(err.status).toBe(404);
  });
});

describe("api – error response with unparseable body", () => {
  beforeEach(() => mockFetch.mockReset());

  it("throws with fallback message and ERR_UNKNOWN code", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("bad gateway", { status: 503 }),
    );
    const err = await api.getBalance("tok").catch((e) => e);
    expect(err.message).toBe("HTTP 503");
    expect(err.code).toBe("ERR_UNKNOWN");
    expect(err.status).toBe(503);
  });
});

describe("api – 204 No Content", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns empty object", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    const result = await api.leaveGroup("tok", 42);
    expect(result).toEqual({});
  });
});

describe("api – getLedger with cursor and limit", () => {
  beforeEach(() => mockFetch.mockReset());

  it("includes cursor and limit in query string", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.getLedger("tok", "abc123", 25);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("cursor=abc123");
    expect(String(url)).toContain("limit=25");
  });
});

describe("api – getLedger without cursor", () => {
  beforeEach(() => mockFetch.mockReset());

  it("does not include cursor= param", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.getLedger("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).not.toContain("cursor=");
  });
});

describe("api – POST with JSON body (submitPrediction)", () => {
  beforeEach(() => mockFetch.mockReset());

  it("uses method=POST and sends JSON body", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 1, match_id: 10, home_score: 2, away_score: 1 }),
    );
    await api.submitPrediction("tok", {
      match_id: 10,
      home_score: 2,
      away_score: 1,
    });
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      match_id: 10,
      home_score: 2,
      away_score: 1,
    });
  });
});

describe("api – PATCH (updateMe)", () => {
  beforeEach(() => mockFetch.mockReset());

  it("uses method=PATCH", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 1, display_name: "New Name" }),
    );
    await api.updateMe("tok", { display_name: "New Name" });
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("PATCH");
  });
});

describe("api – FormData upload (uploadKYCDocument)", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends FormData body without Content-Type in custom headers", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 5, status: "pending" }));
    const fd = new FormData();
    fd.append("file", new Blob(["data"], { type: "image/jpeg" }), "doc.jpg");
    await api.uploadKYCDocument("tok", fd);
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).body).toBe(fd);
    // FormData requests must NOT set Content-Type manually (browser sets multipart boundary)
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Content-Type"]).toBeUndefined();
  });
});

describe("api – createPaymentIntent", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends Idempotency-Key header", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: "pi_abc", status: "created" }),
    );
    await api.createPaymentIntent(
      "tok",
      { amount_cents: 1000, currency: "GTQ", provider: "test" },
      "idem_key_xyz",
    );
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("idem_key_xyz");
  });
});

describe("api – adminGetKYCQueue with status filter", () => {
  beforeEach(() => mockFetch.mockReset());

  it("includes status in query string", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminGetKYCQueue("tok", "pending");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("status=pending");
  });
});

// ── Additional API method coverage ───────────────────────────────────────────

describe("api – group methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("getMyGroups sends GET to /api/v1/groups/me", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getMyGroups("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/me");
  });

  it("getGroup sends GET to /api/v1/groups/:id", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 5 }));
    await api.getGroup("tok", 5);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/5");
  });

  it("createGroup sends POST with name", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 7, name: "Mi Grupo" }));
    await api.createGroup("tok", { name: "Mi Grupo" });
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      name: "Mi Grupo",
    });
  });

  it("joinGroup sends POST to /api/v1/groups/join", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.joinGroup("tok", "INVITE123");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/join");
    expect((init as RequestInit).method).toBe("POST");
  });

  it("joinGroupWithBalance sends POST to /api/v1/groups/join-with-balance", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.joinGroupWithBalance("tok", "INVITE123");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/join-with-balance");
  });

  it("leaveGroup sends DELETE", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.leaveGroup("tok", 3);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/3/members/me");
    expect((init as RequestInit).method).toBe("DELETE");
  });

  it("getGroupLeaderboard includes breakdown param when requested", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        entries: [],
        active_paid_members: 0,
        winner_count: 0,
        eligible_for_prizes: false,
      }),
    );
    await api.getGroupLeaderboard("tok", 1, true);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("breakdown=true");
  });

  it("getGroupMembers sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getGroupMembers("tok", 1);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/1/members");
  });

  it("approveGroupMember sends POST to members/:id/approve", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 99, status: "active" }));
    await api.approveGroupMember("tok", 1, 99);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/1/members/99/approve");
    expect((init as RequestInit).method).toBe("POST");
  });

  it("rejectGroupMember sends DELETE to members/:id", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.rejectGroupMember("tok", 1, 99);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/1/members/99");
    expect((init as RequestInit).method).toBe("DELETE");
  });

  it("getGroupLivePredictions sends GET to /api/v1/groups/:id/live-predictions", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ live_matches: [], user_predictions: [] }),
    );
    await api.getGroupLivePredictions("tok", 7);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/7/live-predictions");
  });
});

describe("api – match and prediction methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("getMatches sends GET to /api/v1/matches", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getMatches("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/matches");
  });

  it("getMyPredictions sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getMyPredictions("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/predictions/me");
  });

  it("updatePrediction sends PATCH", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1 }));
    await api.updatePrediction("tok", 1, { home_score: 1, away_score: 0 });
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("PATCH");
  });
});

describe("api – KYC methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("getKYCRequirements sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ documents: [] }));
    await api.getKYCRequirements("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/kyc/requirements");
  });

  it("getKYCDocuments sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getKYCDocuments("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/kyc/documents");
  });

  it("getKYCEvents without cursor sends GET", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ events: [], next_cursor: "" }),
    );
    await api.getKYCEvents("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/kyc/events");
    expect(String(url)).not.toContain("cursor");
  });

  it("getKYCEvents with cursor appends ?cursor=", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ events: [], next_cursor: "" }),
    );
    await api.getKYCEvents("tok", "cur_xyz");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("cursor=cur_xyz");
  });

  it("submitKYC sends POST", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, status: "pending" }));
    await api.submitKYC("tok", {
      full_name: "Test",
      date_of_birth: "1990-01-01",
      nationality: "GT",
      document_type: "passport",
      document_number: "AB123",
      address_line: "Calle 1",
      city: "Guatemala",
      country: "GT",
      postal_code: "01001",
    });
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
  });
});

describe("api – payment and withdrawal methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("uploadBankTransfer sends FormData with idempotency key", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, status: "pending" }));
    const fd = new FormData();
    fd.append("receipt", new Blob(["img"], { type: "image/jpeg" }), "r.jpg");
    await api.uploadBankTransfer("tok", fd, "idem_key_bt");
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).body).toBe(fd);
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("idem_key_bt");
  });

  it("getMyBankTransfers sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getMyBankTransfers("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/bank-transfers");
  });

  it("createWithdrawal sends POST with idempotency key", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1 }));
    await api.createWithdrawal(
      "tok",
      {
        amount_cents: 500,
        currency: "GTQ",
        method: "bank_gt",
        payout_details: {
          bank_name: "Test",
          account_number: "123",
          account_type: "Ahorros GTQ",
          account_holder: "Test User",
        },
      },
      "idem_wdl",
    );
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("idem_wdl");
  });

  it("getMyWithdrawals sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getMyWithdrawals("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/withdrawals");
  });
});

describe("api – notification methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("getInbox sends GET with limit and offset", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ items: [], total: 0 }));
    await api.getInbox("tok", 20, 10);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("limit=20");
    expect(String(url)).toContain("offset=10");
  });

  it("markRead sends POST with ids", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.markRead("tok", [1, 2, 3]);
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      ids: [1, 2, 3],
    });
  });

  it("markAllRead sends POST with mark_all", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.markAllRead("tok");
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      mark_all: true,
    });
  });

  it("getPreferences sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.getPreferences("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/notifications/preferences");
  });
});

describe("api – admin methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("adminGetStats sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ users: 0 }));
    await api.adminGetStats("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/stats");
  });

  it("adminApproveKYC sends POST with default tier 2", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 1, status: "approved" }),
    );
    await api.adminApproveKYC("tok", 42);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/profiles/42/approve");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      tier: 2,
    });
  });

  it("adminApproveKYC sends POST with explicit tier", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 1, status: "approved" }),
    );
    await api.adminApproveKYC("tok", 42, 3);
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      tier: 3,
    });
  });

  it("adminListUsers without params sends GET with only limit", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.adminListUsers("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/users");
    expect(String(url)).toContain("limit=50");
    expect(String(url)).not.toContain("search=");
    expect(String(url)).not.toContain("banned=");
    expect(String(url)).not.toContain("role=");
    expect(String(url)).not.toContain("cursor=");
  });

  it("adminListUsers with all params appends each to query string", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.adminListUsers("tok", {
      search: "alice",
      banned: true,
      role: "admin",
      cursor: "cur_abc",
    });
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("search=alice");
    expect(String(url)).toContain("banned=true");
    expect(String(url)).toContain("role=admin");
    expect(String(url)).toContain("cursor=cur_abc");
  });

  it("adminGetUserProfile sends GET to /api/v1/admin/users/:id", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ user: { id: 7 }, memberships: [], payments: [] }),
    );
    await api.adminGetUserProfile("tok", 7);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/users/7");
  });

  it("adminBanUser sends POST with reason body", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 7, banned_at: "2026-01-01" }),
    );
    await api.adminBanUser("tok", 7, "policy violation");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/users/7/ban");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      reason: "policy violation",
    });
  });

  it("adminUnbanUser sends DELETE to /api/v1/admin/users/:id/ban", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 7, banned_at: null }));
    await api.adminUnbanUser("tok", 7);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/users/7/ban");
    expect((init as RequestInit).method).toBe("DELETE");
  });

  it("adminSetUserRole sends PATCH with role body", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 7, role: "admin" }));
    await api.adminSetUserRole("tok", 7, "admin");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/users/7/role");
    expect((init as RequestInit).method).toBe("PATCH");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      role: "admin",
    });
  });

  it("adminRejectKYC sends POST with reason", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 1, status: "rejected" }),
    );
    await api.adminRejectKYC("tok", 42, "Docs expired");
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      reason: "Docs expired",
    });
  });

  it("adminGetExchangeRate sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ sell_rate: "7.80" }));
    await api.adminGetExchangeRate("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/exchange-rate/current");
  });

  it("adminGetExchangeRateHistory without cursor sends GET", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.adminGetExchangeRateHistory("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/exchange-rate/history");
    expect(String(url)).not.toContain("cursor=");
  });

  it("adminGetExchangeRateHistory with cursor appends ?cursor=", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.adminGetExchangeRateHistory("tok", "cur_hist");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("cursor=cur_hist");
  });

  it("adminOverrideExchangeRate sends POST", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ sell_rate: "7.90" }));
    await api.adminOverrideExchangeRate("tok", {
      reference_rate: "7.85",
      reason: "Manual",
    });
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
  });

  it("adminRefreshExchangeRate sends POST", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ sell_rate: "7.80" }));
    await api.adminRefreshExchangeRate("tok");
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
  });

  it("adminGetSSEStats sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ connections: 0 }));
    await api.adminGetSSEStats("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/notifications/sse/stats");
  });

  it("adminGetSystemParams sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminGetSystemParams("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/system-params");
  });

  it("adminGetScoringRules sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminGetScoringRules("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/scoring-rules");
  });

  it("adminGetCircuitBreakers sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminGetCircuitBreakers("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain(
      "/api/v1/admin/observability/circuit-breakers",
    );
  });

  it("adminGetKYCQueue without params sends GET with empty query string", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ data: [], next_cursor: "", has_more: false }),
    );
    await api.adminGetKYCQueue("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/kyc/queue");
  });
});

describe("api – deleteKYCDocument", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends DELETE to /api/v1/kyc/documents/:id", async () => {
    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await api.deleteKYCDocument("tok_del", 42);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/kyc/documents/42");
    expect((init as RequestInit).method).toBe("DELETE");
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok_del");
  });
});

describe("api – getBanks", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends GET to /api/v1/banks", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([{ id: 1, name: "BAC" }]));
    const result = await api.getBanks("tok_banks");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/banks");
    expect(Array.isArray(result)).toBe(true);
  });
});

describe("api – getBankAccountTypes", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends GET to /api/v1/bank-account-types", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse([{ id: 1, name: "Monetaria" }]),
    );
    const result = await api.getBankAccountTypes("tok_types");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/bank-account-types");
    expect(Array.isArray(result)).toBe(true);
  });
});

describe("api – createPayPalOrder", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends POST to /api/v1/paypal/create-order", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ order_id: "ord_abc123" }));
    await api.createPayPalOrder("tok", { amount_cents: 500, currency: "USD" });
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/paypal/create-order");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      amount_cents: 500,
      currency: "USD",
    });
  });
});

describe("api – getWithdrawalLimits", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends GET to /api/v1/withdrawals/limits", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ min_gtq_cents: 3000, min_usd_cents: 400 }),
    );
    await api.getWithdrawalLimits("tok_lim");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/withdrawals/limits");
  });
});

describe("api – admin bank and account-type methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("adminListBanks without filter sends GET to /api/v1/admin/banks", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListBanks("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/banks");
    expect(String(url)).not.toContain("active=true");
  });

  it("adminListBanks with onlyActive=true appends ?active=true", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListBanks("tok", true);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/banks?active=true");
  });

  it("adminCreateBank sends POST with name body", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 1, name: "My Bank", active: true }),
    );
    await api.adminCreateBank("tok", "My Bank");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/banks");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      name: "My Bank",
    });
  });

  it("adminSetBankActive sends PATCH with active flag", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 5, name: "Bank", active: false }),
    );
    await api.adminSetBankActive("tok", 5, false);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/banks/5/active");
    expect((init as RequestInit).method).toBe("PATCH");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      active: false,
    });
  });

  it("adminListBankAccountTypes without filter sends GET", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListBankAccountTypes("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/bank-account-types");
    expect(String(url)).not.toContain("active=true");
  });

  it("adminListBankAccountTypes with onlyActive=true appends ?active=true", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListBankAccountTypes("tok", true);
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain(
      "/api/v1/admin/bank-account-types?active=true",
    );
  });

  it("adminCreateBankAccountType sends POST with name body", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 2, name: "Ahorros GTQ", active: true }),
    );
    await api.adminCreateBankAccountType("tok", "Ahorros GTQ");
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      name: "Ahorros GTQ",
    });
  });

  it("adminSetBankAccountTypeActive sends PATCH", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 3, name: "Ahorros GTQ", active: true }),
    );
    await api.adminSetBankAccountTypeActive("tok", 3, true);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/bank-account-types/3/active");
    expect((init as RequestInit).method).toBe("PATCH");
  });
});

// ── Admin: match methods ──────────────────────────────────────────────────────

describe("api – adminStartMatch", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends POST to /api/v1/matches/:id/start", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 5, status: "in_progress" }),
    );
    await api.adminStartMatch("tok", 5);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/matches/5/start");
    expect((init as RequestInit).method).toBe("POST");
  });
});

describe("api – adminCancelMatch", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends POST to /api/v1/matches/:id/cancel with no body", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 7, status: "cancelled" }),
    );
    await api.adminCancelMatch("tok", 7);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/matches/7/cancel");
    expect((init as RequestInit).method).toBe("POST");
  });
});

describe("api – adminCorrectMatchResult", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends POST to /api/v1/matches/:id/correct-result with score data", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 4, home_score: 3, away_score: 1 }),
    );
    await api.adminCorrectMatchResult("tok", 4, {
      home_score: 3,
      away_score: 1,
    });
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/matches/4/correct-result");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      home_score: 3,
      away_score: 1,
    });
  });

  it("includes win_method when provided", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        id: 4,
        home_score: 1,
        away_score: 0,
        win_method: "extra_time",
      }),
    );
    await api.adminCorrectMatchResult("tok", 4, {
      home_score: 1,
      away_score: 0,
      win_method: "extra_time",
    });
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      home_score: 1,
      away_score: 0,
      win_method: "extra_time",
    });
  });
});

describe("api – checkGroupName", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends GET to /api/v1/groups/check-name with name param", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ available: true }));
    const result = await api.checkGroupName("tok", "MiGrupo");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/check-name");
    expect(String(url)).toContain("name=MiGrupo");
    expect(result.available).toBe(true);
  });

  it("returns available=false when name is taken", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ available: false }));
    const result = await api.checkGroupName("tok", "Ocupado");
    expect(result.available).toBe(false);
  });
});

describe("api – adminUpdateMatchResult", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends PATCH to /api/v1/matches/:id with score data", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 3, home_score: 2, away_score: 1 }),
    );
    await api.adminUpdateMatchResult("tok", 3, {
      home_score: 2,
      away_score: 1,
    });
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/matches/3");
    expect((init as RequestInit).method).toBe("PATCH");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      home_score: 2,
      away_score: 1,
    });
  });

  it("includes win_method when provided", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        id: 3,
        home_score: 1,
        away_score: 0,
        win_method: "penalties",
      }),
    );
    await api.adminUpdateMatchResult("tok", 3, {
      home_score: 1,
      away_score: 0,
      win_method: "penalties",
    });
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      home_score: 1,
      away_score: 0,
      win_method: "penalties",
    });
  });
});

// ── Admin: bank-transfer methods ──────────────────────────────────────────────

describe("api – admin bank-transfer methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("adminListBankTransfers sends GET to /api/v1/admin/bank-transfers", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListBankTransfers("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/bank-transfers");
  });

  it("adminApproveBankTransfer sends POST with notes and approved_amount_cents", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 5, status: "approved" }),
    );
    await api.adminApproveBankTransfer("tok", 5, {
      notes: "ok",
      approved_amount_cents: 10000,
    });
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/bank-transfers/5/approve");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      notes: "ok",
      approved_amount_cents: 10000,
    });
  });

  it("adminApproveBankTransfer sends POST with empty data object", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 5, status: "approved" }),
    );
    await api.adminApproveBankTransfer("tok", 5, {});
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({});
  });

  it("adminRejectBankTransfer sends POST with notes", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 7, status: "rejected" }),
    );
    await api.adminRejectBankTransfer("tok", 7, "invalid proof");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/bank-transfers/7/reject");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      notes: "invalid proof",
    });
  });
});

// ── Admin: withdrawal methods ─────────────────────────────────────────────────

describe("api – admin withdrawal methods", () => {
  beforeEach(() => mockFetch.mockReset());

  it("adminListWithdrawals without status omits query string", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListWithdrawals("tok");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/withdrawals");
    expect(String(url)).not.toContain("?status=");
  });

  it("adminListWithdrawals with status appends ?status= query", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse([]));
    await api.adminListWithdrawals("tok", "pending");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/withdrawals?status=pending");
  });

  it("adminApproveWithdrawal with notes sends notes in body", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 3, status: "approved" }),
    );
    await api.adminApproveWithdrawal("tok", 3, "all good");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/withdrawals/3/approve");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      notes: "all good",
    });
  });

  it("adminApproveWithdrawal without notes sends empty string", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 3, status: "approved" }),
    );
    await api.adminApproveWithdrawal("tok", 3);
    const [, init] = mockFetch.mock.calls[0];
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      notes: "",
    });
  });

  it("adminRejectWithdrawal sends POST with notes", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 4, status: "rejected" }),
    );
    await api.adminRejectWithdrawal("tok", 4, "fraud suspected");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/withdrawals/4/reject");
    expect((init as RequestInit).method).toBe("POST");
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      notes: "fraud suspected",
    });
  });

  it("adminProcessWithdrawal sends POST to /process", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ id: 6, status: "processed" }),
    );
    await api.adminProcessWithdrawal("tok", 6);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/withdrawals/6/process");
    expect((init as RequestInit).method).toBe("POST");
  });
});

// ── serverAPI ─────────────────────────────────────────────────────────────────

describe("serverAPI()", () => {
  it("throws when BACKEND_INTERNAL_URL is unset", () => {
    const orig = process.env.BACKEND_INTERNAL_URL;
    delete process.env.BACKEND_INTERNAL_URL;
    expect(() => serverAPI()).toThrow("BACKEND_INTERNAL_URL is not set");
    process.env.BACKEND_INTERNAL_URL = orig;
  });

  it("returns a client when BACKEND_INTERNAL_URL is set", () => {
    process.env.BACKEND_INTERNAL_URL = "http://backend:8080";
    const client = serverAPI();
    expect(client).toBeDefined();
    expect(typeof client.getExchangeRate).toBe("function");
  });
});

// ── getServerToken ────────────────────────────────────────────────────────────

describe("getServerToken()", () => {
  it("returns token from Clerk auth()", async () => {
    const mockGetToken = vi.fn().mockResolvedValue("server_tok_abc");
    vi.mocked(auth).mockResolvedValue({ getToken: mockGetToken } as never);
    const token = await getServerToken();
    expect(token).toBe("server_tok_abc");
  });

  it("returns null when Clerk returns null", async () => {
    const mockGetToken = vi.fn().mockResolvedValue(null);
    vi.mocked(auth).mockResolvedValue({ getToken: mockGetToken } as never);
    const token = await getServerToken();
    expect(token).toBeNull();
  });
});

describe("api – adminTriggerDailySync", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends POST to /api/v1/admin/match-sync/today with no date params", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ total: 10, updated: 2 }));
    await api.adminTriggerDailySync("tok");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/match-sync/today");
    expect(String(url)).not.toContain("start_date");
    expect((init as RequestInit).method).toBe("POST");
  });

  it("appends start_date and end_date query params when provided", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ total: 5, updated: 1 }));
    await api.adminTriggerDailySync("tok", "2026-06-14", "2026-06-14");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("start_date=2026-06-14");
    expect(String(url)).toContain("end_date=2026-06-14");
  });

  it("omits query string when only startDate is provided", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ total: 3, updated: 0 }));
    await api.adminTriggerDailySync("tok", "2026-06-10");
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("start_date=2026-06-10");
    expect(String(url)).not.toContain("end_date");
  });
});

// ── adminGetMetricsSummary ─────────────────────────────────────────────────────

describe("api – adminGetMetricsSummary", () => {
  beforeEach(() => mockFetch.mockReset());

  it("calls the metrics/summary endpoint with Bearer token", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        configured: true,
        request_rate_per_sec: 1.5,
        error_rate_5xx: 0.01,
        p95_latency_seconds: 0.12,
      }),
    );
    const result = await api.adminGetMetricsSummary("tok_admin");
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/observability/metrics/summary");
    expect((init as RequestInit).headers as Record<string, string>).toMatchObject({
      Authorization: "Bearer tok_admin",
    });
    expect(result.configured).toBe(true);
    expect(result.request_rate_per_sec).toBe(1.5);
  });
});

// ── adminQueryMetrics ─────────────────────────────────────────────────────────

describe("api – adminQueryMetrics", () => {
  beforeEach(() => mockFetch.mockReset());

  it("encodes the PromQL expression and sends it as the q= query parameter", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ configured: true, results: [] }),
    );
    await api.adminQueryMetrics("tok_admin", "rate(http_requests_total[5m])");
    const [url] = mockFetch.mock.calls[0];
    // Brackets are percent-encoded; parentheses may remain literal — both are valid.
    const urlStr = String(url);
    expect(urlStr).toContain("/api/v1/admin/observability/metrics/query");
    expect(urlStr).toContain("http_requests_total");
    expect(urlStr).toMatch(/[?&]q=/);
  });

  it("returns results array from the response", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        configured: true,
        results: [{ labels: { job: "api" }, value: 3.14 }],
      }),
    );
    const result = await api.adminQueryMetrics("tok", "up");
    expect(result.results).toHaveLength(1);
    expect(result.results[0].value).toBe(3.14);
  });
});

// ── adminSearchLogs ───────────────────────────────────────────────────────────

describe("api – adminSearchLogs", () => {
  beforeEach(() => mockFetch.mockReset());

  it("calls the logs endpoint with no params when all are omitted", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ configured: true, errors: [] }),
    );
    await api.adminSearchLogs("tok_admin", {});
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/admin/observability/logs");
    expect(String(url)).not.toContain("q=");
    expect(String(url)).not.toContain("since=");
    expect(String(url)).not.toContain("limit=");
  });

  it("appends q, since, and limit when provided", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({ configured: true, errors: [] }),
    );
    await api.adminSearchLogs("tok", { q: "paypal", since: 60, limit: 25 });
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("q=paypal");
    expect(String(url)).toContain("since=60");
    expect(String(url)).toContain("limit=25");
  });

  it("returns errors array from the response", async () => {
    mockFetch.mockResolvedValueOnce(
      makeResponse({
        configured: true,
        errors: [
          {
            trace_id: "abc123",
            root_service_name: "api",
            root_trace_name: "POST /paypal",
            start_time_unix_nano: "1700000000000000000",
            duration_ms: 120,
          },
        ],
      }),
    );
    const result = await api.adminSearchLogs("tok", { q: "paypal" });
    expect(result.errors).toHaveLength(1);
    expect(result.errors[0].trace_id).toBe("abc123");
  });
});

describe("api.updateScoreFromZero", () => {
  beforeEach(() => mockFetch.mockReset());

  it("sends PATCH to /api/v1/groups/{id}/score-from-zero with score_from_zero flag", async () => {
    mockFetch.mockResolvedValueOnce(makeResponse({ id: 1, score_from_zero: true }));
    await api.updateScoreFromZero("tok_abc", 42, true);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/api/v1/groups/42/score-from-zero");
    expect((init as RequestInit).method).toBe("PATCH");
    const body = JSON.parse((init as RequestInit).body as string);
    expect(body.score_from_zero).toBe(true);
  });
});
