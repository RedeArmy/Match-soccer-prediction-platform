// @vitest-environment node
import { describe, it, expect, vi, beforeEach } from "vitest";

// Hoist Clerk mock
vi.mock("@clerk/nextjs/server", () => ({ auth: vi.fn() }));

// Mock next/server so NextResponse/NextRequest work in node env without full Next.js runtime
vi.mock("next/server", () => {
  class MockNextResponse extends Response {
    static json(data: unknown, init?: ResponseInit) {
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
        ...(init?.headers != null
          ? (init.headers as Record<string, string>)
          : {}),
      };
      return new MockNextResponse(JSON.stringify(data), { ...init, headers });
    }
  }

  class MockNextRequest extends Request {
    nextUrl: URL;
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(input, init);
      this.nextUrl = new URL(
        typeof input === "string"
          ? input
          : input instanceof URL
            ? input.href
            : (input as Request).url,
      );
    }
  }

  return {
    NextResponse: MockNextResponse,
    NextRequest: MockNextRequest,
  };
});

import { auth } from "@clerk/nextjs/server";
import { NextRequest } from "next/server";

// ── Fetch mock ────────────────────────────────────────────────────────────────

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

// Use the mocked NextRequest so .nextUrl is available
function makeReq(
  url = "http://localhost/api/v1/matches",
  init?: RequestInit,
): NextRequest {
  return new NextRequest(
    url,
    init as ConstructorParameters<typeof NextRequest>[1],
  );
}

// ── health/route ──────────────────────────────────────────────────────────────

describe("GET /api/health", () => {
  it("returns { status: ok, service: frontend }", async () => {
    const { GET } = await import("@/app/api/health/route");
    const res = await GET();
    const body = await res.json();
    expect(body).toEqual({ status: "ok", service: "frontend" });
  });
});

// ── [...path]/route (BFF proxy) ───────────────────────────────────────────────

type Context = { params: Promise<{ path: string[] }> };

function makeCtx(path: string[] = ["v1", "matches"]): Context {
  return { params: Promise.resolve({ path }) };
}

describe("[...path] proxy – GET without Clerk token", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("calls upstream fetch without Authorization header", async () => {
    mockFetch.mockResolvedValueOnce(new Response("[]", { status: 200 }));
    const { GET } = await import("@/app/api/[...path]/route");
    await GET(makeReq(), makeCtx());
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBeUndefined();
  });
});

describe("[...path] proxy – GET with client Authorization header", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it("forwards Authorization header from the incoming request", async () => {
    mockFetch.mockResolvedValueOnce(new Response("[]", { status: 200 }));
    const { GET } = await import("@/app/api/[...path]/route");
    const req = makeReq("http://localhost/api/v1/matches", {
      headers: { Authorization: "Bearer clerk_token_abc" },
    });
    await GET(req, makeCtx());
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer clerk_token_abc");
  });
});

describe("[...path] proxy – POST request", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("calls upstream with body from request.arrayBuffer()", async () => {
    mockFetch.mockResolvedValueOnce(new Response("{}", { status: 201 }));
    const { POST } = await import("@/app/api/[...path]/route");
    const bodyStr = JSON.stringify({ home_score: 1, away_score: 2 });
    const req = makeReq("http://localhost/api/v1/predictions", {
      method: "POST",
      body: bodyStr,
      headers: { "Content-Type": "application/json" },
    });
    await POST(req, makeCtx(["v1", "predictions"]));
    const [, init] = mockFetch.mock.calls[0];
    expect((init as RequestInit).method).toBe("POST");
    expect((init as RequestInit).body).toBeDefined();
  });
});

describe("[...path] proxy – upstream fetch throws", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("returns 502 with ERR_UPSTREAM code", async () => {
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { GET } = await import("@/app/api/[...path]/route");
    const res = await GET(makeReq(), makeCtx());
    expect(res.status).toBe(502);
    const body = await res.json();
    expect(body.error.code).toBe("ERR_UPSTREAM");
  });
});

describe("[...path] proxy – forwards Idempotency-Key header", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("forwards Idempotency-Key from incoming request to upstream", async () => {
    mockFetch.mockResolvedValueOnce(new Response("{}", { status: 201 }));
    const { POST } = await import("@/app/api/[...path]/route");
    const req = makeReq("http://localhost/api/v1/withdrawals", {
      method: "POST",
      body: "{}",
      headers: { "Idempotency-Key": "idem_abc_123" },
    });
    await POST(req, makeCtx(["v1", "withdrawals"]));
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("idem_abc_123");
  });
});

describe("[...path] proxy – upstream non-OK response is proxied", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("returns the upstream status code when upstream responds with 4xx", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { code: "NOT_FOUND", message: "not found" } }),
        {
          status: 404,
          headers: { "content-type": "application/json" },
        },
      ),
    );
    const { GET } = await import("@/app/api/[...path]/route");
    const res = await GET(makeReq(), makeCtx());
    expect(res.status).toBe(404);
  });
});

describe("[...path] proxy – hop-by-hop header stripping", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("strips connection header from proxied response", async () => {
    const upstream = new Response("{}", {
      status: 200,
      headers: { connection: "keep-alive", "x-custom": "foo" },
    });
    mockFetch.mockResolvedValueOnce(upstream);
    const { GET } = await import("@/app/api/[...path]/route");
    const res = await GET(makeReq(), makeCtx());
    expect(res.headers.get("connection")).toBeNull();
  });

  it("forwards non-hop-by-hop headers", async () => {
    const upstream = new Response("{}", {
      status: 200,
      headers: { connection: "keep-alive", "x-custom": "foo" },
    });
    mockFetch.mockResolvedValueOnce(upstream);
    const { GET } = await import("@/app/api/[...path]/route");
    const res = await GET(makeReq(), makeCtx());
    expect(res.headers.get("x-custom")).toBe("foo");
  });
});

// ── webhooks/clerk/route ──────────────────────────────────────────────────────

describe("clerk webhook relay – happy path", () => {
  beforeEach(() => mockFetch.mockReset());

  it("forwards request body to backend and returns upstream status", async () => {
    mockFetch.mockResolvedValueOnce(new Response("{}", { status: 200 }));
    const { POST } = await import("@/app/webhooks/clerk/route");
    const req = makeReq("http://localhost/webhooks/clerk", {
      method: "POST",
      body: JSON.stringify({ type: "user.created" }),
      headers: {
        "content-type": "application/json",
        "svix-id": "msg_abc",
        "svix-timestamp": "1234567890",
        "svix-signature": "v1,abc123",
      },
    });
    const res = await POST(req);
    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledOnce();
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/webhooks/clerk");
  });

  it("strips hop-by-hop headers from the upstream response", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("{}", {
        status: 200,
        headers: { "transfer-encoding": "chunked", "x-request-id": "abc" },
      }),
    );
    const { POST } = await import("@/app/webhooks/clerk/route");
    const req = makeReq("http://localhost/webhooks/clerk", {
      method: "POST",
      body: "{}",
    });
    const res = await POST(req);
    expect(res.headers.get("transfer-encoding")).toBeNull();
    expect(res.headers.get("x-request-id")).toBe("abc");
  });
});

describe("clerk webhook relay – hop-by-hop header stripping in request", () => {
  beforeEach(() => mockFetch.mockReset());

  it("strips hop-by-hop headers from forwarded request", async () => {
    mockFetch.mockResolvedValueOnce(new Response("{}", { status: 200 }));
    const { POST } = await import("@/app/webhooks/clerk/route");
    const req = makeReq("http://localhost/webhooks/clerk", {
      method: "POST",
      body: "{}",
      headers: {
        connection: "keep-alive",
        "x-custom-header": "preserved",
      },
    });
    await POST(req);
    const [, init] = mockFetch.mock.calls[0];
    const headers = (init as RequestInit).headers as Record<string, string>;
    expect(headers["connection"]).toBeUndefined();
    expect(headers["x-custom-header"]).toBe("preserved");
  });
});

describe("clerk webhook relay – upstream fetch throws", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns 502 when backend is unreachable", async () => {
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { POST } = await import("@/app/webhooks/clerk/route");
    const req = makeReq("http://localhost/webhooks/clerk", {
      method: "POST",
      body: "{}",
    });
    const res = await POST(req);
    expect(res.status).toBe(502);
    const body = await res.json();
    expect(body.error).toBe("Backend unavailable");
  });
});

// ── notifications/stream/route ────────────────────────────────────────────────

// ── webhooks/paypal/route ─────────────────────────────────────────────────────

describe("paypal webhook relay – happy path", () => {
  beforeEach(() => mockFetch.mockReset());

  it("forwards request body to backend and returns upstream status", async () => {
    process.env.BACKEND_INTERNAL_URL = "http://backend:8080";
    mockFetch.mockResolvedValueOnce(new Response("{}", { status: 200 }));
    const { POST } = await import("@/app/webhooks/paypal/route");
    const req = makeReq("http://localhost/webhooks/paypal", {
      method: "POST",
      body: JSON.stringify({ event_type: "PAYMENT.CAPTURE.COMPLETED" }),
      headers: {
        "content-type": "application/json",
        "paypal-transmission-id": "txn_abc",
      },
    });
    const res = await POST(req);
    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledOnce();
    const [url] = mockFetch.mock.calls[0];
    expect(String(url)).toContain("/webhooks/paypal");
  });
});

describe("paypal webhook relay – strips hop-by-hop headers", () => {
  beforeEach(() => mockFetch.mockReset());

  it("strips connection header from proxied response", async () => {
    process.env.BACKEND_INTERNAL_URL = "http://backend:8080";
    mockFetch.mockResolvedValueOnce(
      new Response("{}", {
        status: 200,
        headers: { connection: "keep-alive", "x-request-id": "req_1" },
      }),
    );
    const { POST } = await import("@/app/webhooks/paypal/route");
    const req = makeReq("http://localhost/webhooks/paypal", {
      method: "POST",
      body: "{}",
    });
    const res = await POST(req);
    expect(res.headers.get("connection")).toBeNull();
    expect(res.headers.get("x-request-id")).toBe("req_1");
  });
});

describe("paypal webhook relay – upstream fetch throws", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns 502 when backend is unreachable", async () => {
    process.env.BACKEND_INTERNAL_URL = "http://backend:8080";
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { POST } = await import("@/app/webhooks/paypal/route");
    const req = makeReq("http://localhost/webhooks/paypal", {
      method: "POST",
      body: "{}",
    });
    const res = await POST(req);
    expect(res.status).toBe(502);
    const body = await res.json();
    expect(body.error).toBe("Backend unavailable");
  });
});

describe("SSE stream proxy – no token", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue(null),
    } as never);
  });

  it("returns 401", async () => {
    const { GET } = await import("@/app/api/notifications/stream/route");
    const res = await GET();
    expect(res.status).toBe(401);
    const body = await res.json();
    expect(body.error.code).toBe("ERR_UNAUTHORISED");
  });
});

describe("SSE stream proxy – upstream throws", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue("tok_sse"),
    } as never);
  });

  it("returns 502", async () => {
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { GET } = await import("@/app/api/notifications/stream/route");
    const res = await GET();
    expect(res.status).toBe(502);
    const body = await res.json();
    expect(body.error.code).toBe("ERR_UPSTREAM");
  });
});

describe("SSE stream proxy – upstream responds", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    vi.mocked(auth).mockResolvedValue({
      getToken: vi.fn().mockResolvedValue("tok_sse"),
    } as never);
  });

  it("proxies body with content-type: text/event-stream", async () => {
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: {}\n\n"));
        controller.close();
      },
    });
    mockFetch.mockResolvedValueOnce(new Response(stream, { status: 200 }));
    const { GET } = await import("@/app/api/notifications/stream/route");
    const res = await GET();
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toContain("text/event-stream");
  });
});

// ── live/today/route ──────────────────────────────────────────────────────────

describe("GET /api/live/today – no API key", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    delete process.env.FOOTBALL_API_KEY;
  });

  it("returns { fixtures: [] } gracefully when key is absent", async () => {
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toEqual([]);
  });
});

const clockOkResponse = () =>
  new Response(JSON.stringify({ now: "2026-06-11T12:00:00Z" }), {
    status: 200,
  });

const emptyAfResponse = () =>
  new Response(JSON.stringify({ results: 0, errors: [], response: [] }), {
    status: 200,
  });

describe("GET /api/live/today – upstream OK", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_live_today";
    mockFetch.mockResolvedValueOnce(clockOkResponse());
  });

  it("maps api-football response to TodayFixture array", async () => {
    const afPayload = {
      results: 1,
      errors: [],
      response: [
        {
          fixture: {
            id: 42,
            date: "2026-06-10T20:00:00Z",
            status: { short: "1H", elapsed: 30 },
            venue: { name: "Estadio X", city: null },
          },
          league: { round: "Group Stage - 1" },
          teams: {
            home: { id: 1, name: "Brazil", logo: "https://logo/br.png" },
            away: { id: 2, name: "Germany", logo: "https://logo/de.png" },
          },
          goals: { home: 1, away: 0 },
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(afPayload), { status: 200 }),
    );
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // tomorrow — two-day fetch
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toHaveLength(1);
    expect(body.fixtures[0].id).toBe(42);
    expect(body.fixtures[0].homeTeam).toBe("Brazil");
    expect(body.fixtures[0].status).toBe("1H");
    expect(body.fixtures[0].elapsed).toBe(30);
    expect(body.fixtures[0].homeScore).toBe(1);
    expect(body.fixtures[0].venue).toBe("Estadio X");
  });
});

describe("GET /api/live/today – upstream fetch throws", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_live_today";
    mockFetch.mockResolvedValueOnce(clockOkResponse());
  });

  it("returns { fixtures: [] } on network error", async () => {
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED")); // today throws
    mockFetch.mockResolvedValueOnce(emptyAfResponse());          // tomorrow
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toEqual([]);
  });
});

describe("GET /api/live/today – upstream non-OK", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_live_today";
    mockFetch.mockResolvedValueOnce(clockOkResponse());
  });

  it("returns { fixtures: [] } on upstream 429", async () => {
    mockFetch.mockResolvedValueOnce(new Response("", { status: 429 })); // today
    mockFetch.mockResolvedValueOnce(emptyAfResponse());                  // tomorrow
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toEqual([]);
  });
});

describe("GET /api/live/today – empty response array", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_live_today";
    mockFetch.mockResolvedValueOnce(clockOkResponse());
  });

  it("returns { fixtures: [] } when no matches scheduled today", async () => {
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // today
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // tomorrow
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toEqual([]);
  });
});

describe("GET /api/live/today – clock non-OK falls back to current date", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_live_today";
    mockFetch.mockResolvedValueOnce(new Response("", { status: 503 }));
  });

  it("still returns fixtures when clock returns non-OK status", async () => {
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // today (clock already consumed)
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // tomorrow
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toEqual([]);
  });
});

describe("GET /api/live/today – clock fetch throws falls back to current date", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_live_today";
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
  });

  it("still returns fixtures when clock fetch throws", async () => {
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // today (clock rejected was already consumed)
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // tomorrow
    const { GET } = await import("@/app/api/live/today/route");
    const res = await GET(new Request("http://localhost/api/live/today"));
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixtures).toEqual([]);
  });
});

describe("GET /api/live/today – client date param used when valid", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_client_date";
    // Two parallel fetches: today (2026-06-14) and tomorrow (2026-06-15).
    // No clock fetch when a valid client date is provided.
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // today
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // tomorrow
  });

  it("uses browser-supplied date instead of backend clock", async () => {
    const { GET } = await import("@/app/api/live/today/route");
    const req = new Request("http://localhost/api/live/today?date=2026-06-14");
    const res = await GET(req);
    expect(res.status).toBe(200);
    // The first upstream fetch URL should contain the client-supplied date.
    const [upstreamUrl] = mockFetch.mock.calls[0];
    expect(String(upstreamUrl)).toContain("date=2026-06-14");
  });
});

describe("GET /api/live/today – invalid client date falls back to clock", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_invalid_date";
    // clock → today's af → tomorrow's af (three fetches when client date is invalid)
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ now: "2026-06-14T10:00:00Z" }), {
        status: 200,
      }),
    );
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // today
    mockFetch.mockResolvedValueOnce(emptyAfResponse()); // tomorrow
  });

  it("ignores malformed date param and calls backend clock", async () => {
    const { GET } = await import("@/app/api/live/today/route");
    const req = new Request("http://localhost/api/live/today?date=not-a-date");
    const res = await GET(req);
    expect(res.status).toBe(200);
    // Three fetch calls: clock + today af + tomorrow af (clientDate was invalid)
    expect(mockFetch.mock.calls).toHaveLength(3);
  });
});

// ── live/fixture/[id]/route ───────────────────────────────────────────────────

type FixtureContext = { params: Promise<{ id: string }> };
function makeFixtureCtx(id: string): FixtureContext {
  return { params: Promise.resolve({ id }) };
}

describe("GET /api/live/fixture/[id] – no API key", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    delete process.env.FOOTBALL_API_KEY;
  });

  it("returns 503 when FOOTBALL_API_KEY is absent", async () => {
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/42"),
      makeFixtureCtx("42"),
    );
    expect(res.status).toBe(503);
  });
});

describe("GET /api/live/fixture/[id] – invalid id", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("returns 400 for non-numeric id", async () => {
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/abc"),
      makeFixtureCtx("abc"),
    );
    expect(res.status).toBe(400);
  });

  it("returns 400 for zero id", async () => {
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/0"),
      makeFixtureCtx("0"),
    );
    expect(res.status).toBe(400);
  });

  it("returns 400 for negative id", async () => {
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/-5"),
      makeFixtureCtx("-5"),
    );
    expect(res.status).toBe(400);
  });

  it("returns 400 for float id", async () => {
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/42.5"),
      makeFixtureCtx("42.5"),
    );
    expect(res.status).toBe(400);
  });
});

describe("GET /api/live/fixture/[id] – upstream OK", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("returns mapped FixtureDetail on success", async () => {
    const afPayload = {
      response: [
        {
          fixture: {
            id: 42,
            date: "2026-06-10T20:00:00Z",
            status: { short: "1H", elapsed: 35 },
            venue: { name: "AT&T Stadium", city: "Arlington" },
          },
          league: { round: "Group Stage - 1" },
          teams: {
            home: { name: "Brazil", logo: "https://logo/br.png" },
            away: { name: "Germany", logo: "https://logo/de.png" },
          },
          goals: { home: 1, away: 0 },
          score: { halftime: { home: null, away: null } },
          lineups: [
            {
              team: { name: "Brazil" },
              formation: "4-3-3",
              startXI: [
                { player: { id: 1, name: "Alisson", number: 1, pos: "G" } },
              ],
              substitutes: [],
            },
          ],
          events: [
            {
              time: { elapsed: 22, extra: null },
              team: { name: "Brazil" },
              player: { name: "Vinicius Jr." },
              assist: { name: "Rodrygo" },
              type: "Goal",
              detail: "Normal Goal",
            },
          ],
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(afPayload), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/42"),
      makeFixtureCtx("42"),
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixture.id).toBe(42);
    expect(body.fixture.homeTeam).toBe("Brazil");
    expect(body.fixture.lineups).toHaveLength(1);
    expect(body.fixture.lineups[0].formation).toBe("4-3-3");
    expect(body.fixture.events).toHaveLength(1);
    expect(body.fixture.events[0].player).toBe("Vinicius Jr.");
    expect(body.fixture.events[0].assist).toBe("Rodrygo");
  });
});

describe("GET /api/live/fixture/[id] – upstream fetch throws", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("returns 502 on network error", async () => {
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/99"),
      makeFixtureCtx("99"),
    );
    expect(res.status).toBe(502);
  });
});

describe("GET /api/live/fixture/[id] – upstream non-OK", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("proxies upstream status code on error", async () => {
    mockFetch.mockResolvedValueOnce(new Response("", { status: 503 }));
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/5"),
      makeFixtureCtx("5"),
    );
    expect(res.status).toBe(503);
  });
});

describe("GET /api/live/fixture/[id] – empty response array", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("returns 404 when api-football returns no items", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ response: [] }), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/1"),
      makeFixtureCtx("1"),
    );
    expect(res.status).toBe(404);
  });
});

describe("GET /api/live/fixture/[id] – null inner fields (pre-match)", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("handles null startXI / substitutes inside lineups without crashing", async () => {
    const payload = {
      response: [
        {
          fixture: {
            id: 99,
            date: "2026-06-20T20:00:00Z",
            status: { short: "NS", elapsed: null },
            venue: null,
          },
          league: { round: "Group Stage - 3" },
          teams: {
            home: { name: "USA", logo: "" },
            away: { name: "Mexico", logo: "" },
          },
          goals: { home: null, away: null },
          score: null,
          lineups: [
            {
              team: { name: "USA" },
              formation: null,
              startXI: null,
              substitutes: null,
            },
          ],
          events: null,
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/99"),
      makeFixtureCtx("99"),
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixture.id).toBe(99);
    expect(body.fixture.venue).toBeNull();
    expect(body.fixture.halftimeHome).toBeNull();
    expect(body.fixture.lineups[0].startXI).toEqual([]);
    expect(body.fixture.lineups[0].substitutes).toEqual([]);
    expect(body.fixture.events).toEqual([]);
  });
});

// ── live/fixture/[id]/route – edge cases ─────────────────────────────────────

describe("GET /api/live/fixture/[id] – null assist and halftime numbers", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("maps null assist to null in the assist field", async () => {
    // Covers the ev.assist?.name ?? null branch where assist is null.
    const payload = {
      response: [
        {
          fixture: {
            id: 200,
            date: "2026-06-15T20:00:00Z",
            status: { short: "FT", elapsed: 90 },
            venue: { name: "Stadium X", city: "City" },
          },
          league: { round: "Quarter-final" },
          teams: {
            home: { name: "Spain", logo: "" },
            away: { name: "Portugal", logo: "" },
          },
          goals: { home: 2, away: 1 },
          score: { halftime: { home: 1, away: 0 } },
          lineups: [],
          events: [
            {
              time: { elapsed: 34, extra: null },
              team: { name: "Spain" },
              player: { name: "Morata" },
              assist: null, // null assist — must not throw
              type: "Goal",
              detail: "Normal Goal",
            },
          ],
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/200"),
      makeFixtureCtx("200"),
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixture.events[0].assist).toBeNull();
    // Halftime values are numbers (covers the non-null halftime branch).
    expect(body.fixture.halftimeHome).toBe(1);
    expect(body.fixture.halftimeAway).toBe(0);
  });

  it("maps venue name null inside venue object to null", async () => {
    // Covers item.fixture.venue?.name ?? null when venue exists but name is null.
    const payload = {
      response: [
        {
          fixture: {
            id: 201,
            date: "2026-06-16T20:00:00Z",
            status: { short: "NS", elapsed: null },
            venue: { name: null, city: "Unknown City" },
          },
          league: { round: "Group Stage - 1" },
          teams: {
            home: { name: "Japan", logo: "" },
            away: { name: "Korea", logo: "" },
          },
          goals: { home: null, away: null },
          score: { halftime: { home: null, away: null } },
          lineups: [],
          events: [],
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/201"),
      makeFixtureCtx("201"),
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixture.venue).toBeNull();
  });
});

describe("GET /api/live/fixture/[id] – event with missing optional fields", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("falls back to defaults for null event fields", async () => {
    // Covers the ?? fallback branches in events.map:
    // ev.time?.elapsed ?? 0, ev.team?.name ?? "", ev.player?.name ?? "",
    // ev.type ?? "", ev.detail ?? ""
    const payload = {
      response: [
        {
          fixture: {
            id: 203,
            date: "2026-06-18T20:00:00Z",
            status: { short: "HT", elapsed: 45 },
            venue: { name: "Rose Bowl", city: "Pasadena" },
          },
          league: { round: "Semi-final" },
          teams: {
            home: { name: "France", logo: "" },
            away: { name: "England", logo: "" },
          },
          goals: { home: 0, away: 0 },
          score: { halftime: { home: 0, away: 0 } },
          lineups: [],
          events: [
            {
              time: null,
              team: null,
              player: null,
              assist: null,
              type: null,
              detail: null,
            },
          ],
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/203"),
      makeFixtureCtx("203"),
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixture.events).toHaveLength(1);
    const ev = body.fixture.events[0];
    expect(ev.elapsed).toBe(0);
    expect(ev.team).toBe("");
    expect(ev.player).toBe("");
    expect(ev.type).toBe("");
    expect(ev.detail).toBe("");
  });
});

describe("GET /api/live/fixture/[id] – lineups with substitutes", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    process.env.FOOTBALL_API_KEY = "test_key_fixture";
  });

  it("maps substitute players in lineup", async () => {
    const payload = {
      response: [
        {
          fixture: {
            id: 202,
            date: "2026-06-17T20:00:00Z",
            status: { short: "2H", elapsed: 60 },
            venue: { name: "Estadio Azteca", city: "Mexico City" },
          },
          league: { round: "Group Stage - 2" },
          teams: {
            home: { name: "Mexico", logo: "" },
            away: { name: "USA", logo: "" },
          },
          goals: { home: 1, away: 1 },
          score: { halftime: { home: 1, away: 0 } },
          lineups: [
            {
              team: { name: "Mexico" },
              formation: "4-4-2",
              startXI: [
                {
                  player: { id: 10, name: "Memo Ochoa", number: 13, pos: "G" },
                },
              ],
              substitutes: [
                {
                  player: { id: 20, name: "Raul Jimenez", number: 9, pos: "F" },
                },
              ],
            },
          ],
          events: [],
        },
      ],
    };
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(payload), { status: 200 }),
    );
    const { GET } = await import("@/app/api/live/fixture/[id]/route");
    const res = await GET(
      makeReq("http://localhost/api/live/fixture/202"),
      makeFixtureCtx("202"),
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.fixture.lineups[0].substitutes).toHaveLength(1);
    expect(body.fixture.lineups[0].substitutes[0].name).toBe("Raul Jimenez");
    expect(body.fixture.lineups[0].substitutes[0].number).toBe(9);
    expect(body.fixture.lineups[0].substitutes[0].pos).toBe("F");
  });
});

// ── geo/route ─────────────────────────────────────────────────────────────────

describe("GET /api/geo – private / loopback IP returns GT", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns GT for 127.x loopback without calling ipapi.co", async () => {
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "127.0.0.1" },
    });
    const res = await GET(req as never);
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.country).toBe("GT");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns GT for 10.x private IP without calling ipapi.co", async () => {
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "10.0.0.5" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("GT");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns GT when X-Forwarded-For header is absent", async () => {
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo");
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("GT");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns GT for 192.168.x private IP", async () => {
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "192.168.1.100" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("GT");
    expect(mockFetch).not.toHaveBeenCalled();
  });
});

describe("GET /api/geo – public IP calls ipapi.co", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns country_code from ipapi.co for a public IP", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ country_code: "US" }), { status: 200 }),
    );
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "8.8.8.8" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("US");
    expect(mockFetch).toHaveBeenCalledOnce();
    const [calledUrl] = mockFetch.mock.calls[0];
    expect(String(calledUrl)).toContain("8.8.8.8");
  });

  it("uses the first IP when X-Forwarded-For contains multiple addresses", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ country_code: "MX" }), { status: 200 }),
    );
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "201.100.1.5, 10.0.0.1" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("MX");
    const [calledUrl] = mockFetch.mock.calls[0];
    expect(String(calledUrl)).toContain("201.100.1.5");
  });

  it("falls back to GT when ipapi.co returns non-OK status", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("Service Unavailable", { status: 503 }),
    );
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "8.8.8.8" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("GT");
  });

  it("falls back to GT when ipapi.co fetch throws", async () => {
    mockFetch.mockRejectedValueOnce(new Error("network timeout"));
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "8.8.8.8" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("GT");
  });

  it("falls back to GT when ipapi.co returns no country_code field", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ ip: "8.8.8.8" }), { status: 200 }),
    );
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "8.8.8.8" },
    });
    const res = await GET(req as never);
    const body = await res.json();
    expect(body.country).toBe("GT");
  });

  it("sets Cache-Control: private, max-age=3600", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify({ country_code: "GT" }), { status: 200 }),
    );
    const { GET } = await import("@/app/api/geo/route");
    const req = makeReq("http://localhost/api/geo", {
      headers: { "x-forwarded-for": "190.5.10.20" },
    });
    const res = await GET(req as never);
    expect(res.headers.get("Cache-Control")).toBe("private, max-age=3600");
  });
});

// ── /api/public/standings/route ───────────────────────────────────────────────

describe("GET /api/public/standings – upstream success", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns matches from upstream", async () => {
    const matches = [
      { id: 1, home_team: "Brazil", away_team: "Germany", home_score: null, away_score: null, status: "scheduled", group_label: "A" },
    ];
    mockFetch.mockResolvedValueOnce(
      new Response(JSON.stringify(matches), { status: 200 }),
    );
    const { GET } = await import("@/app/api/public/standings/route");
    const res = await GET();
    const body = await res.json();
    expect(body.matches).toHaveLength(1);
    expect(body.matches[0].home_team).toBe("Brazil");
  });
});

describe("GET /api/public/standings – upstream non-ok", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns empty matches when upstream responds non-ok", async () => {
    mockFetch.mockResolvedValueOnce(new Response("", { status: 503 }));
    const { GET } = await import("@/app/api/public/standings/route");
    const res = await GET();
    const body = await res.json();
    expect(body.matches).toEqual([]);
  });
});

describe("GET /api/public/standings – upstream throws", () => {
  beforeEach(() => mockFetch.mockReset());

  it("returns empty matches when fetch throws", async () => {
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { GET } = await import("@/app/api/public/standings/route");
    const res = await GET();
    const body = await res.json();
    expect(body.matches).toEqual([]);
  });
});
