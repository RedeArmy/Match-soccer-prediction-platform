"use client";

import { useExchangeRate } from "@/hooks/useExchangeRate";
import { useI18n } from "@/lib/i18n";
import { formatGTQ, formatUSD, gtqToUSD, usdToGTQ } from "@/lib/utils";

const FALLBACK_SELL_RATE = "7.75";
const FALLBACK_BUY_RATE = "7.75";

/**
 * Returns a `fmt(cents)` function that formats a balance amount in the
 * currency appropriate for the current locale and the user's balance_currency.
 *
 * balanceCurrency: the currency in which `cents` is denominated ("GTQ" or "USD").
 *
 * Formatting rules:
 *   - GTQ balance, Spanish locale: show GTQ directly
 *   - GTQ balance, English locale: convert GTQ→USD via sell rate
 *   - USD balance, English locale: show USD directly (exact, no conversion)
 *   - USD balance, Spanish locale: convert USD→GTQ via buy rate
 */
export function useCurrency(balanceCurrency = "GTQ") {
  const { locale } = useI18n();
  const { data: rate } = useExchangeRate();
  const isUSD = locale === "en";
  const sellRate = rate?.sell_rate ?? FALLBACK_SELL_RATE;
  const buyRate = rate?.buy_rate ?? FALLBACK_BUY_RATE;

  function fmt(cents: number): string {
    if (balanceCurrency === "USD") {
      if (isUSD) {
        return formatUSD(cents);
      }
      return formatGTQ(Math.round(usdToGTQ(cents / 100, buyRate) * 100));
    }
    if (isUSD) {
      return formatUSD(Math.round(gtqToUSD(cents / 100, sellRate) * 100));
    }
    return formatGTQ(cents);
  }

  return { fmt, isUSD };
}
