// Webhook relay — forwards PayPal webhook events to the Go backend unchanged.
// PayPal RSA signature headers (PAYPAL-TRANSMISSION-ID, PAYPAL-TRANSMISSION-TIME,
// PAYPAL-CERT-URL, PAYPAL-TRANSMISSION-SIG, PAYPAL-AUTH-ALGO) must reach the
// backend intact for certificate verification; we must NOT add an Authorization
// header and must NOT buffer-transform the body.
import { NextRequest, NextResponse } from "next/server";
import { relayWebhook } from "@/lib/webhook-relay";

const PAYPAL_SIGNATURE_HEADERS = [
  "PAYPAL-TRANSMISSION-ID",
  "PAYPAL-TRANSMISSION-TIME",
  "PAYPAL-CERT-URL",
  "PAYPAL-TRANSMISSION-SIG",
  "PAYPAL-AUTH-ALGO",
];

export function POST(req: NextRequest): Promise<NextResponse> {
  return relayWebhook(req, "/webhooks/paypal", "paypal webhook relay", PAYPAL_SIGNATURE_HEADERS);
}
