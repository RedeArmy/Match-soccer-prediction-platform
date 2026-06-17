import { NextRequest, NextResponse } from "next/server";

const PRIVATE_PREFIXES = ["127.", "10.", "172.16.", "172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168.", "::1", "fc", "fd"];

function isPrivateIP(ip: string): boolean {
  return PRIVATE_PREFIXES.some((prefix) => ip.startsWith(prefix));
}

export async function GET(req: NextRequest): Promise<NextResponse> {
  const forwarded = req.headers.get("x-forwarded-for");
  const ip = forwarded?.split(",")[0]?.trim() ?? "";

  if (!ip || isPrivateIP(ip)) {
    // Dev / loopback — default to Guatemala so local testing shows GT behaviour.
    return respond("GT");
  }

  try {
    const upstream = await fetch(`https://ipapi.co/${ip}/json/`, {
      headers: { "User-Agent": "WCQ/1.0 geo-lookup" },
      signal: AbortSignal.timeout(3000),
    });
    if (!upstream.ok) throw new Error(`ipapi.co ${upstream.status}`);
    const data = (await upstream.json()) as { country_code?: string };
    return respond(data.country_code ?? "GT");
  } catch (err) {
    console.error("[geo] lookup failed", ip, err);
    return respond("GT");
  }
}

function respond(country: string): NextResponse {
  const res = NextResponse.json({ country });
  // Cache 1 hour in the browser; no shared cache (response is per-user).
  res.headers.set("Cache-Control", "private, max-age=3600");
  return res;
}
