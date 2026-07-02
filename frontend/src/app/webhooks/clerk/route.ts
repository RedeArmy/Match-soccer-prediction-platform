// Webhook relay — forwards Clerk webhook events to the Go backend unchanged.
// Svix signature headers (svix-id, svix-timestamp, svix-signature) must reach
// the backend intact for HMAC verification; we must NOT add an Authorization
// header and must NOT buffer-transform the body.
import { NextRequest, NextResponse } from "next/server";
import { relayWebhook } from "@/lib/webhook-relay";

const SVIX_HEADERS = ["svix-id", "svix-timestamp", "svix-signature"];

export function POST(req: NextRequest): Promise<NextResponse> {
  return relayWebhook(req, "/webhooks/clerk", "webhook relay", SVIX_HEADERS);
}
