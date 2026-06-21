import { NextRequest, NextResponse } from "next/server";

const PRIVATE_PREFIXES = ["127.", "10.", "172.16.", "172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168.", "::1", "fc", "fd"];

function isPrivateIP(ip: string): boolean {
  return PRIVATE_PREFIXES.some((prefix) => ip.startsWith(prefix));
}

// ip-api.com — free HTTP endpoint, 45 req/min, higher accuracy for Latin American ISPs.
// Returns { "countryCode": "GT", "status": "success" } or { "status": "fail" }.
async function lookupIpApi(ip: string): Promise<string | null> {
  try {
    const res = await fetch(`http://ip-api.com/json/${ip}?fields=countryCode,status`, {
      headers: { "User-Agent": "WCQ/1.0 geo-lookup" },
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { status?: string; countryCode?: string };
    if (data.status !== "success" || !data.countryCode) return null;
    return data.countryCode;
  } catch {
    return null;
  }
}

// ipapi.co — HTTPS fallback. Free tier: 1 000 req/day without a key.
// Returns { "country_code": "GT", ... } on success.
async function lookupIpapiCo(ip: string): Promise<string | null> {
  try {
    const res = await fetch(`https://ipapi.co/${ip}/json/`, {
      headers: { "User-Agent": "WCQ/1.0 geo-lookup" },
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { country_code?: string };
    return data.country_code ?? null;
  } catch {
    return null;
  }
}

export async function GET(req: NextRequest): Promise<NextResponse> {
  const forwarded = req.headers.get("x-forwarded-for");
  const ip = forwarded?.split(",")[0]?.trim() ?? "";

  if (!ip || isPrivateIP(ip)) {
    // Dev / loopback — default to Guatemala so local testing shows GT behaviour.
    return respond("GT");
  }

  // Try ip-api.com first (better accuracy for Latin American ISPs such as Claro and Tigo
  // Guatemala, which use IP blocks registered to international parent companies).
  const country =
    (await lookupIpApi(ip)) ??
    (await lookupIpapiCo(ip)) ??
    "GT";

  if (country === "GT" || !country) {
    return respond("GT");
  }

  console.info("[geo] detected", { ip: ip.slice(0, 8) + "…", country });
  return respond(country);
}

function respond(country: string): NextResponse {
  const res = NextResponse.json({ country });
  // Cache 1 hour in the browser; no shared cache (response is per-user).
  res.headers.set("Cache-Control", "private, max-age=3600");
  return res;
}
