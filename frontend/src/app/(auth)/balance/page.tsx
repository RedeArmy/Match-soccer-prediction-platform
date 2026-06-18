"use client";

import { useState, useRef, useEffect, useMemo } from "react";
import { useSearchParams } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { api } from "@/lib/api";
import { useCurrency } from "@/hooks/useCurrency";
import { useExchangeRate } from "@/hooks/useExchangeRate";
import { BalanceCard } from "@/components/balance/BalanceCard";
import { LoadingState } from "@/components/shared/LoadingState";
import {
  formatDate,
  formatGTQ,
  usdToGTQ,
  ledgerKindKey,
  isVisibleLedgerKind,
  formatRelative,
} from "@/lib/utils";
import type {
  LedgerEntry,
  PaymentIntentSummary,
  BankTransferResponse,
  PaymentIntentStatus,
} from "@/lib/api-types";
import { useI18n } from "@/lib/i18n";
import { CheckCircle, X, Upload, AlertCircle } from "lucide-react";

// ── Helpers ───────────────────────────────────────────────────────────────────

function getLedgerColor(kind: string, deltaCents: number): string {
  if (kind === "withdrawal_release") return "text-amber-400";
  return deltaCents >= 0 ? "text-green-400" : "text-red-400";
}

function formatAmount(cents: number, currency: string): string {
  if (currency === "USD") return `$${(cents / 100).toFixed(2)} USD`;
  return formatGTQ(cents);
}

type IntentColorKey = "green" | "orange" | "red";

function intentColor(status: PaymentIntentStatus): IntentColorKey {
  if (status === "captured") return "green";
  if (status === "pending" || status === "under_review") return "orange";
  return "red"; // rejected | expired
}

type TransferColorKey = "green" | "orange" | "red";

function transferColor(status: string): TransferColorKey {
  if (status === "approved") return "green";
  if (status === "pending") return "orange";
  return "red";
}

const colorDot: Record<string, string> = {
  green: "bg-emerald-400",
  orange: "bg-amber-400",
  red: "bg-red-400",
};

// ── Unified transaction row types ────────────────────────────────────────────

type TxRow =
  | { kind: "intent"; data: PaymentIntentSummary; date: string }
  | { kind: "transfer"; data: BankTransferResponse; date: string };

// ── Payment row component ─────────────────────────────────────────────────────

function IntentRow({ intent }: { readonly intent: PaymentIntentSummary }) {
  const { t } = useI18n();
  const color = intentColor(intent.status);
  const providerLabel =
    intent.provider === "recurrente" ? "Recurrente" : "PayPal";
  const amount = formatAmount(intent.amount_cents, intent.currency);

  const showAdjuntar =
    (intent.status === "pending" || intent.status === "expired") &&
    !intent.has_comprobante &&
    intent.comprobante_required;

  const showResolucion = intent.status === "rejected";

  return (
    <div className="flex items-center justify-between gap-3 px-4 py-3">
      <div className="flex items-center gap-3 min-w-0">
        <span
          className={`w-2.5 h-2.5 rounded-full shrink-0 ${colorDot[color]}`}
        />
        <div className="min-w-0">
          <p className="text-sm text-text-primary">
            {providerLabel} —{" "}
            <span className="font-score">{amount}</span>
          </p>
          <p className="text-xs text-text-muted">
            {t(`status.${intent.status}`)} ·{" "}
            {formatRelative(intent.created_at)}
          </p>
          {intent.status === "under_review" && (
            <p className="text-xs text-amber-400/80 mt-0.5">
              {t("balance.underReviewMsg")}
            </p>
          )}
          {intent.status === "pending" &&
            intent.comprobante_required &&
            !intent.has_comprobante && (
              <p className="text-xs text-amber-400/80 mt-0.5">
                {t("balance.comprobanteRequiredMsg")}
              </p>
            )}
          {intent.user_notes && intent.status === "under_review" && (
            <p className="text-xs text-text-muted/70 italic truncate max-w-[220px] mt-0.5">
              "{intent.user_notes}"
            </p>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2 shrink-0">
        {showAdjuntar && (
          <Link
            href={`/balance/deposit/comprobante/${encodeURIComponent(intent.token)}`}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-500/15 hover:bg-amber-500/25 text-amber-400 text-xs font-medium transition-colors"
          >
            <Upload className="w-3.5 h-3.5" />
            {t("balance.btnAdjuntar")}
          </Link>
        )}
        {showResolucion && (
          <Link
            href={`/balance/deposit/review/${encodeURIComponent(intent.token)}`}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-red-500/15 hover:bg-red-500/25 text-red-400 text-xs font-medium transition-colors"
          >
            <AlertCircle className="w-3.5 h-3.5" />
            {t("balance.btnResolucion")}
          </Link>
        )}
      </div>
    </div>
  );
}

function TransferRow({ transfer }: { readonly transfer: BankTransferResponse }) {
  const { t } = useI18n();
  const color = transferColor(transfer.status);
  const amount = formatGTQ(transfer.approved_amount_cents ?? transfer.amount_cents);

  return (
    <div className="flex items-center justify-between gap-3 px-4 py-3">
      <div className="flex items-center gap-3 min-w-0">
        <span
          className={`w-2.5 h-2.5 rounded-full shrink-0 ${colorDot[color]}`}
        />
        <div className="min-w-0">
          <p className="text-sm text-text-primary">
            {t("balance.providerTransfer")} —{" "}
            <span className="font-score">{amount}</span>
          </p>
          <p className="text-xs text-text-muted">
            {t(`status.${transfer.status}`)} ·{" "}
            {formatRelative(transfer.created_at)}
          </p>
          {transfer.notes && transfer.status !== "pending" && (
            <p className="text-xs text-text-muted/70 italic truncate max-w-[220px] mt-0.5">
              "{transfer.notes}"
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function BalancePage() {
  const { getToken } = useAuth();
  const { fmt, isUSD } = useCurrency();
  const { t } = useI18n();
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Success redirect params
  const searchParams = useSearchParams();
  const paypalSuccess = searchParams.get("paypal") === "success";
  const recurrenteSuccess = searchParams.get("deposit") === "success";
  const anySuccess = paypalSuccess || recurrenteSuccess;

  const usdAmount = paypalSuccess ? Number(searchParams.get("usd") ?? "0") : 0;
  const recurrenteAmountCents = recurrenteSuccess
    ? Number(searchParams.get("amount") ?? "0")
    : 0;
  const recurrenteCurrency = recurrenteSuccess
    ? (searchParams.get("currency") ?? "GTQ")
    : "GTQ";

  const [bannerVisible, setBannerVisible] = useState(anySuccess);
  const [refetchEnabled, setRefetchEnabled] = useState(anySuccess);
  const { data: rate } = useExchangeRate();

  const gtqEstimate =
    rate && usdAmount > 0
      ? Math.round(usdToGTQ(usdAmount, rate.buy_rate) * 100)
      : null;

  const recurrenteAmountDisplay =
    recurrenteCurrency === "GTQ"
      ? `Q ${(recurrenteAmountCents / 100).toFixed(2)}`
      : `$${(recurrenteAmountCents / 100).toFixed(2)} ${recurrenteCurrency}`;

  const recurrenteAmountFormatted =
    recurrenteAmountCents > 0 ? recurrenteAmountDisplay : null;

  useEffect(() => {
    if (!anySuccess) return;
    const timer = setTimeout(() => setRefetchEnabled(false), 15_000);
    return () => clearTimeout(timer);
  }, [anySuccess]);

  // Payment intents (all statuses)
  const { data: myIntents = [], isLoading: intentsLoading } = useQuery<
    PaymentIntentSummary[]
  >({
    queryKey: ["payment-intents", "my-all"],
    queryFn: async () => {
      const token = await getToken();
      return api.listMyIntents(token!);
    },
    refetchInterval: refetchEnabled ? 3_000 : false,
  });

  // Bank transfers
  const { data: myTransfers = [], isLoading: transfersLoading } = useQuery<
    BankTransferResponse[]
  >({
    queryKey: ["bank-transfers", "my"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMyBankTransfers(token!);
    },
  });

  // Merge and sort by date
  const transactions = useMemo<TxRow[]>(() => {
    const rows: TxRow[] = [
      ...myIntents.map((d) => ({
        kind: "intent" as const,
        data: d,
        date: d.created_at,
      })),
      ...myTransfers.map((d) => ({
        kind: "transfer" as const,
        data: d,
        date: d.created_at,
      })),
    ];
    return rows.sort(
      (a, b) => new Date(b.date).getTime() - new Date(a.date).getTime(),
    );
  }, [myIntents, myTransfers]);

  const txLoading = intentsLoading || transfersLoading;

  // Ledger (infinite scroll)
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } =
    useInfiniteQuery({
      queryKey: ["ledger"],
      initialPageParam: undefined as string | undefined,
      queryFn: async ({ pageParam }) => {
        const token = await getToken();
        return api.getLedger(token!, pageParam, 50);
      },
      getNextPageParam: () => undefined,
      refetchInterval: refetchEnabled ? 2_000 : false,
    });

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { threshold: 0.5 },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const allEntries = (data?.pages.flat() ?? []).filter((e) =>
    isVisibleLedgerKind(e.kind),
  );

  const bannerCreditText = paypalSuccess
    ? t("balance.paypalBannerCredit")
    : t("balance.recurrenteBannerCredit");

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">{t("balance.title")}</h1>

      {/* Payment success banner */}
      {bannerVisible && (
        <div className="flex items-start gap-3 p-4 bg-green-900/50 border border-green-700/50 rounded-xl">
          <CheckCircle className="w-5 h-5 text-green-400 shrink-0 mt-0.5" />
          <div className="min-w-0 flex-1">
            {paypalSuccess && (
              <>
                <p className="text-sm font-medium text-green-300">
                  {t("balance.paypalBannerPre")}{" "}
                  <span className="font-score">${usdAmount.toFixed(2)} USD</span>{" "}
                  {t("balance.paypalBannerReceived")}
                </p>
                {gtqEstimate !== null && (
                  <p className="text-xs text-green-400/80 mt-0.5">
                    ≈ {formatGTQ(gtqEstimate)} GTQ{" "}
                    {t("balance.paypalBannerRate")}
                  </p>
                )}
              </>
            )}
            {recurrenteSuccess && (
              <p className="text-sm font-medium text-green-300">
                {t("balance.recurrenteBannerPre")}{" "}
                {recurrenteAmountFormatted && (
                  <span className="font-score">{recurrenteAmountFormatted}</span>
                )}{" "}
                {t("balance.recurrenteBannerReceived")}
              </p>
            )}
            <p className="text-xs text-text-muted mt-0.5">{bannerCreditText}</p>
          </div>
          <button
            type="button"
            onClick={() => setBannerVisible(false)}
            className="text-text-muted hover:text-text-secondary"
            aria-label={t("balance.bannerClose")}
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      <BalanceCard />

      {/* Payment transactions (unified colored list) */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">
          {t("balance.txTitle")}
        </h2>

        {txLoading && <LoadingState rows={4} />}

        {!txLoading && transactions.length === 0 && (
          <p className="text-text-muted text-sm text-center py-6">
            {t("balance.txEmpty")}
          </p>
        )}

        {!txLoading && transactions.length > 0 && (
          <div className="card divide-y divide-blue-800/50">
            {transactions.map((tx, i) => {
              if (tx.kind === "intent") {
                return <IntentRow key={`intent-${tx.data.id}-${i}`} intent={tx.data} />;
              }
              return <TransferRow key={`transfer-${tx.data.id}-${i}`} transfer={tx.data} />;
            })}
          </div>
        )}
      </section>

      {/* Ledger (internal movements: prizes, fees, withdrawals, etc.) */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">
          {t("balance.historyTitle")}
        </h2>

        {isLoading && <LoadingState rows={6} />}
        {!isLoading && allEntries.length === 0 && (
          <p className="text-text-muted text-sm text-center py-8">
            {t("balance.historyEmpty")}
          </p>
        )}
        {!isLoading && allEntries.length > 0 && (
          <div className="card divide-y divide-blue-800/50">
            {allEntries.map((entry: LedgerEntry) => (
              <div
                key={entry.id}
                className="flex items-center justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="text-sm text-text-primary truncate">
                    {t(ledgerKindKey(entry.kind))}
                  </p>
                  <p className="text-xs text-text-muted">
                    {formatDate(entry.created_at)}
                  </p>
                </div>
                <div className="text-right shrink-0">
                  <p
                    className={`font-score text-sm font-medium ${getLedgerColor(entry.kind, entry.delta_cents)}`}
                  >
                    {entry.delta_cents >= 0 ? "+" : ""}
                    {fmt(entry.delta_cents)}
                  </p>
                  <p className="text-[10px] text-text-muted">
                    {isUSD ? "USD" : "GTQ"}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}

        <div ref={sentinelRef} className="h-4" />
        {isFetchingNextPage && <LoadingState rows={2} />}
      </section>
    </div>
  );
}
