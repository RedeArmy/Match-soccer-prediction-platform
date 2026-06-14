"use client";

import Link from "next/link";
import { ArrowDownCircle, ArrowUpCircle, Wallet } from "lucide-react";
import { useBalance } from "@/hooks/useBalance";
import { useCurrency } from "@/hooks/useCurrency";
import { LoadingState } from "@/components/shared/LoadingState";
import { useI18n } from "@/lib/i18n";

export function BalanceCard() {
  const { data: balance, isLoading } = useBalance();
  const { fmt } = useCurrency();
  const { t } = useI18n();

  if (isLoading) return <LoadingState rows={1} className="h-40" />;

  const available = balance?.available_cents ?? 0;
  const reserved = balance?.reserved_cents ?? 0;
  const pending = balance?.pending_cents ?? 0;

  return (
    <div className="card-elevated space-y-4 p-5">
      <div className="flex items-center gap-2">
        <Wallet className="h-4 w-4 text-gold-400" />
        <span className="text-sm font-semibold text-text-secondary">
          {t("balanceCard.title")}
        </span>
      </div>

      <div className="text-center">
        <p className="mb-0.5 text-xs text-text-muted">
          {t("balanceCard.available")}
        </p>
        <p className="font-score text-3xl font-semibold leading-none text-white">
          {fmt(available)}
        </p>
      </div>

      <div className="flex gap-4 text-xs text-text-muted">
        {reserved > 0 && (
          <div>
            <span className="text-gold-400">{t("balanceCard.reserved")}: </span>
            {fmt(reserved)}
          </div>
        )}
        {pending > 0 && (
          <div>
            <span className="text-blue-300">{t("balanceCard.pending")}: </span>
            {fmt(pending)}
          </div>
        )}
      </div>

      <div className="flex flex-col gap-2 pt-1">
        <Link href="/balance/deposit" className="btn-gold py-2 text-sm">
          <ArrowDownCircle className="h-4 w-4" />
          {t("balanceCard.deposit")}
        </Link>
        <Link href="/balance/withdraw" className="btn-ghost py-2 text-sm">
          <ArrowUpCircle className="h-4 w-4" />
          {t("balanceCard.withdraw")}
        </Link>
      </div>
    </div>
  );
}
