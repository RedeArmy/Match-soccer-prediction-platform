-- Migration 000156: add api_version to exchange_rate_history.
--
-- The source column identifies the provider name ("banguat", "exchangerate-api",
-- "openexchangerates") but not the API revision used to fetch the rate.  If a
-- provider changes its feed format or endpoint version (e.g. v6 → v7 on
-- exchangerate-api.com, or Banguat migrates from XML to JSON) historical rows
-- become indistinguishable from rows fetched under the new schema, making
-- post-incident debugging harder.
--
-- api_version carries the short revision string set by each fetcher at call time:
--   "v6"  — ExchangeRate-API  (https://v6.exchangerate-api.com/v6/...)
--   "v1"  — OpenExchangeRates (/api/latest.json; their API is versioned v1)
--   "xml" — Banguat public XML feed (no URL version; label denotes wire format)
--   ""    — admin overrides and stale-fallback rows (no external API call made)
--
-- NOT NULL DEFAULT '': existing rows and all future override/stale rows receive
-- an empty string, which is the semantically correct value — no provider API
-- was invoked.  Application code sets a non-empty value only when a live
-- external fetch succeeds.

ALTER TABLE exchange_rate_history
    ADD COLUMN api_version VARCHAR(20) NOT NULL DEFAULT '';
