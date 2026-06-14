import { NextRequest, NextResponse } from "next/server";

const HOP_HEADERS = new Set([
  "content-encoding",
  "transfer-encoding",
  "connection",
  "keep-alive",
]);

export async function relayWebhook(
  req: NextRequest,
  backendPath: string,
  logTag: string,
): Promise<NextResponse> {
  const url = `${process.env.BACKEND_INTERNAL_URL!}${backendPath}`;

  const headers: Record<string, string> = {};
  req.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      headers[k] = v;
    }
  });

  const body = await req.arrayBuffer();

  let upstream: Response;
  try {
    upstream = await fetch(url, {
      method: "POST",
      headers,
      body,
      cache: "no-store",
    });
  } catch (err) {
    console.error(`[${logTag}] upstream fetch failed`, err);
    return NextResponse.json({ error: "Backend unavailable" }, { status: 502 });
  }

  const resHeaders = new Headers();
  upstream.headers.forEach((v, k) => {
    if (!HOP_HEADERS.has(k.toLowerCase())) {
      resHeaders.set(k, v);
    }
  });

  return new NextResponse(upstream.body, {
    status: upstream.status,
    headers: resHeaders,
  });
}
