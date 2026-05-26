-- Migration 000120: add ip_address to balance_ledger.
--
-- Captures the originating IP address for every balance mutation so
-- that the compliance team and KYC reviewers can correlate financial
-- events with network identity during risk investigations.
--
-- ip_address is NULL for system/webhook-triggered entries where no
-- client IP is available (e.g. scheduled prize credits, admin bulk
-- adjustments initiated outside an HTTP request context).
--
-- The column stores a raw text representation (IPv4 or IPv6) rather
-- than the PostgreSQL inet type so that application-layer redaction
-- (masking the last octet for GDPR) can be applied before insert.

ALTER TABLE balance_ledger
  ADD COLUMN ip_address TEXT;
