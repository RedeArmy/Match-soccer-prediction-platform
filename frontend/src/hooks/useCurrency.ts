"use client";

import { useExchangeRate } from "@/hooks/useExchangeRate";
import { useI18n } from "@/lib/i18n";
import { formatGTQ, formatUSD, gtqToUSD } from "@/lib/utils";

const FALLBACK_SELL_RATE = "7.75"; // used only while the live rate is loading

/**
 * Returns a `fmt(cents)` function that formats a GTQ-cent amount in the
 * currency that matches the current locale: USD for English, GTQ for Spanish.
 * Conversion uses the live sell-rate; falls back to 7.75 GTQ/USD while loading.
 */
export function useCurrency() {
  const { locale } = useI18n();
  const { data: rate } = useExchangeRate();
  const isUSD = locale === "en";
  const sellRate = rate?.sell_rate ?? FALLBACK_SELL_RATE;

  function fmt(cents: number): string {
    if (isUSD) {
      return formatUSD(Math.round(gtqToUSD(cents / 100, sellRate) * 100));
    }
    return formatGTQ(cents);
  }

  return { fmt, isUSD };
}
