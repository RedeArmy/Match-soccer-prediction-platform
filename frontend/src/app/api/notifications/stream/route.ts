// SSE proxy — streams /api/v1/notifications/stream from the Go backend.
// A dedicated route is required because the generic [...path] proxy buffers
// the response body before returning, which breaks streaming.
// The Clerk JWT is injected server-side via the Authorization header.
import { auth } from "@clerk/nextjs/server";
import { NextResponse } from "next/server";

const BACKEND = process.env.BACKEND_INTERNAL_URL!;

export async function GET(): Promise<NextResponse> {
  const { getToken } = await auth();
  const token = await getToken();

  if (!token) {
    return NextResponse.json(
      {
        error: {
          schema_version: 1,
          code: "ERR_UNAUTHORISED",
          message: "Authentication required",
        },
      },
      { status: 401 },
    );
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${BACKEND}/api/v1/notifications/stream`, {
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: "text/event-stream",
        "Cache-Control": "no-cache",
        "X-Accel-Buffering": "no",
      },
      cache: "no-store",
      // @ts-expect-error — duplex required for streaming in some runtimes
      duplex: "half",
    });
  } catch (err) {
    console.error("[SSE proxy] backend unreachable", err);
    return NextResponse.json(
      {
        error: {
          schema_version: 1,
          code: "ERR_UPSTREAM",
          message: "SSE backend unavailable",
        },
      },
      { status: 502 },
    );
  }

  return new NextResponse(upstream.body, {
    status: upstream.status,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
