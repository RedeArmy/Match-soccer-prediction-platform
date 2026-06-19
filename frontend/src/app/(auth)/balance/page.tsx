"use client";

import { useState, useMemo } from "react";
import { useSearchParams } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useCurrency } from "@/hooks/useCurrency";
import { useBalance } from "@/hooks/useBalance";
import { useExchangeRate } from "@/hooks/useExchangeRate";
import { BalanceCard } from "@/components/balance/BalanceCard";
import { LoadingState } from "@/components/shared/LoadingState";
import {
  formatDate,
  formatGTQ,
  formatUSD,
  usdToGTQ,
  ledgerKindKey,
  isVisibleLedgerKind,
} from "@/lib/utils";
import type { LedgerEntry, BankTransferResponse } from "@/lib/api-types";
import { useI18n } from "@/lib/i18n";
import { CheckCircle, X, ChevronLeft, ChevronRight } from "lucide-react";

// ── Helpers ───────────────────────────────────────────────────────────────────

function getLedgerColor(kind: string, deltaCents: number): string {
  if (kind === "withdrawal_release") return "text-amber-400";
  return deltaCents >= 0 ? "text-green-400" : "text-red-400";
}

function transferColor(status: string): "green" | "orange" | "red" {
  if (status === "approved") return "green";
  if (status === "pending") return "orange";
  return "red";
}

const colorDot: Record<string, string> = {
  green: "bg-emerald-400",
  orange: "bg-amber-400",
  red: "bg-red-400",
};

const statusBadge: Record<string, string> = {
  green: "bg-emerald-500/15 text-emerald-300 ring-emerald-500/25",
  orange: "bg-amber-500/15 text-amber-300 ring-amber-500/25",
  red: "bg-red-500/15 text-red-300 ring-red-500/25",
};

const PAGE_SIZE = 10;

// ── Unified row types ─────────────────────────────────────────────────────────

type TxRow =
  | { kind: "transfer"; data: BankTransferResponse; date: string }
  | { kind: "ledger"; data: LedgerEntry; date: string };

// ── Transfer row ──────────────────────────────────────────────────────────────

function TransferRow({
  transfer,
}: {
  readonly transfer: BankTransferResponse;
}) {
  const { t } = useI18n();
  const color = transferColor(transfer.status);
  const amount = formatGTQ(
    transfer.approved_amount_cents ?? transfer.amount_cents,
  );

  return (
    <div className="flex gap-3 px-4 py-3.5">
      <span
        className={`mt-1.5 w-2.5 h-2.5 rounded-full shrink-0 ${colorDot[color]}`}
      />
      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-medium text-text-primary">
              {t("balance.providerTransfer")}
            </p>
            <p className="text-xs text-text-muted mt-0.5">
              {formatDate(transfer.created_at)}
            </p>
          </div>
          <div className="shrink-0 text-right">
            <p className="font-score text-sm font-semibold text-text-primary">
              {amount}
            </p>
            <span
              className={`inline-block mt-1 px-2 py-0.5 rounded-full text-[10px] font-medium ring-1 ${statusBadge[color]}`}
            >
              {t(`status.${transfer.status}`)}
            </span>
          </div>
        </div>

        {transfer.notes && transfer.status !== "pending" && (
          <p className="text-xs text-text-muted/70 italic mt-1">
            &ldquo;{transfer.notes}&rdquo;
          </p>
        )}
      </div>
    </div>
  );
}

// ── Ledger row ────────────────────────────────────────────────────────────────

function LedgerRow({
  entry,
  fmt,
  isUSD,
}: {
  readonly entry: LedgerEntry;
  readonly fmt: (cents: number) => string;
  readonly isUSD: boolean;
}) {
  const { t } = useI18n();
  const color = getLedgerColor(entry.kind, entry.delta_cents);

  return (
    <div className="flex items-center gap-3 px-4 py-3.5">
      <span className="w-2.5 h-2.5 rounded-full shrink-0 bg-blue-400/50" />
      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm text-text-primary truncate">
              {t(ledgerKindKey(entry.kind))}
            </p>
            <p className="text-xs text-text-muted mt-0.5">
              {formatDate(entry.created_at)}
            </p>
          </div>
          <div className="shrink-0 text-right">
            <p className={`font-score text-sm font-semibold ${color}`}>
              {entry.delta_cents >= 0 ? "+" : ""}
              {isUSD &&
              entry.source_currency === "USD" &&
              entry.source_amount_cents
                ? formatUSD(entry.source_amount_cents)
                : fmt(entry.delta_cents)}
            </p>
            <p className="text-[10px] text-text-muted">
              {isUSD ? "USD" : "GTQ"}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function BalancePage() {
  const { getToken } = useAuth();
  const { data: balanceData } = useBalance();
  const balanceCurrency = balanceData?.balance_currency ?? "GTQ";
  const { fmt, isUSD } = useCurrency(balanceCurrency);
  const { t } = useI18n();

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
  const [page, setPage] = useState(0);

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

  // Ledger entries (fetch a high limit for client-side merge)
  const { data: ledgerEntries = [], isLoading: ledgerLoading } = useQuery<
    LedgerEntry[]
  >({
    queryKey: ["ledger", "all"],
    queryFn: async () => {
      const token = await getToken();
      return api.getLedger(token!, undefined, 500);
    },
    refetchInterval: refetchEnabled ? 2_000 : false,
  });

  // Unified sorted list
  const allRows = useMemo<TxRow[]>(() => {
    const rows: TxRow[] = [
      ...myTransfers.map((d) => ({
        kind: "transfer" as const,
        data: d,
        date: d.created_at,
      })),
      ...ledgerEntries
        .filter((e) => isVisibleLedgerKind(e.kind))
        .map((e) => ({
          kind: "ledger" as const,
          data: e,
          date: e.created_at,
        })),
    ];
    return rows.sort(
      (a, b) => new Date(b.date).getTime() - new Date(a.date).getTime(),
    );
  }, [myTransfers, ledgerEntries]);

  const totalPages = Math.ceil(allRows.length / PAGE_SIZE);
  const pageRows = allRows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const isLoading = transfersLoading || ledgerLoading;

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
                  <span className="font-score">
                    ${usdAmount.toFixed(2)} USD
                  </span>{" "}
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
                  <span className="font-score">
                    {recurrenteAmountFormatted}
                  </span>
                )}{" "}
                {t("balance.recurrenteBannerReceived")}
              </p>
            )}
            <p className="text-xs text-text-muted mt-0.5">{bannerCreditText}</p>
          </div>
          <button
            type="button"
            onClick={() => {
              setBannerVisible(false);
              setRefetchEnabled(false);
            }}
            className="text-text-muted hover:text-text-secondary"
            aria-label={t("balance.bannerClose")}
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      <BalanceCard />

      {/* Unified transaction history */}
      <section className="space-y-3">
        <h2 className="text-sm font-semibold text-text-secondary uppercase tracking-wide">
          {t("balance.historyTitle")}
        </h2>

        {isLoading && <LoadingState rows={6} />}

        {!isLoading && allRows.length === 0 && (
          <p className="text-text-muted text-sm text-center py-8">
            {t("balance.historyEmpty")}
          </p>
        )}

        {!isLoading && allRows.length > 0 && (
          <>
            <div className="card divide-y divide-blue-800/50">
              {pageRows.map((tx, i) => {
                if (tx.kind === "transfer") {
                  return (
                    <TransferRow
                      key={`transfer-${tx.data.id}-${i}`}
                      transfer={tx.data}
                    />
                  );
                }
                return (
                  <LedgerRow
                    key={`ledger-${tx.data.id}-${i}`}
                    entry={tx.data}
                    fmt={fmt}
                    isUSD={isUSD}
                  />
                );
              })}
            </div>

            {totalPages > 1 && (
              <div className="flex items-center justify-between pt-1">
                <p className="text-xs text-text-muted">
                  {page * PAGE_SIZE + 1}–
                  {Math.min((page + 1) * PAGE_SIZE, allRows.length)} /{" "}
                  {allRows.length}
                </p>
                <div className="flex gap-1">
                  <button
                    type="button"
                    disabled={page === 0}
                    onClick={() => setPage((p) => p - 1)}
                    className="p-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-text-muted disabled:opacity-30 transition-colors"
                    aria-label={t("common.prev")}
                  >
                    <ChevronLeft className="w-4 h-4" />
                  </button>
                  <button
                    type="button"
                    disabled={page >= totalPages - 1}
                    onClick={() => setPage((p) => p + 1)}
                    className="p-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-text-muted disabled:opacity-30 transition-colors"
                    aria-label={t("common.next")}
                  >
                    <ChevronRight className="w-4 h-4" />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </section>
    </div>
  );
}
