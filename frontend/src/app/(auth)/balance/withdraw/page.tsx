"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useBalance } from "@/hooks/useBalance";
import { useKYCStatus } from "@/hooks/useKYCStatus";
import { useCurrency } from "@/hooks/useCurrency";
import { useExchangeRate } from "@/hooks/useExchangeRate";
import { useI18n } from "@/lib/i18n";
import { formatGTQ, formatUSD, gtqToUSD } from "@/lib/utils";
import { FormField } from "@/components/shared/FormField";
import { SubmitButton } from "@/components/shared/SubmitButton";
import { CheckCircle2, ShieldAlert, Banknote, Globe, XCircle, X } from "lucide-react";
import Link from "next/link";

type Method = "bank_gt" | "paypal";

function buildPayoutDetails(
  method: Method,
  bankName: string,
  accountType: string,
  accountNumber: string,
  accountHolder: string,
  paypalEmail: string,
): Record<string, string> {
  if (method === "bank_gt") {
    return { bank_name: bankName, account_type: accountType, account_number: accountNumber, account_holder: accountHolder };
  }
  return { paypal_email: paypalEmail };
}

function isWithdrawValid(
  method: Method,
  amountCents: number,
  minCents: number,
  availableDisplay: number,
  paypalEmail: string,
  bankName: string,
  accountType: string,
  accountNumber: string,
  accountHolder: string,
): boolean {
  if (amountCents < minCents || amountCents > availableDisplay) return false;
  if (method === "paypal") return paypalEmail.trim() !== "";
  return bankName !== "" && accountType !== "" && accountNumber.trim() !== "" && accountHolder.trim() !== "";
}

export default function WithdrawPage() {
  const { getToken } = useAuth();
  const { t, accountTypeName } = useI18n();
  const { data: balance } = useBalance();
  const { fmt, isUSD } = useCurrency();
  const { data: rate } = useExchangeRate();
  const sellRate = rate?.sell_rate ?? "7.75";
  const { data: kyc, isLoading: kycLoading } = useKYCStatus();
  const [method, setMethod] = useState<Method>("bank_gt");
  const [amount, setAmount] = useState("");
  const [bankName, setBankName] = useState("");
  const [accountType, setAccountType] = useState("");
  const [accountNumber, setAccountNumber] = useState("");
  const [accountHolder, setAccountHolder] = useState("");
  const [paypalEmail, setPaypalEmail] = useState("");
  const [popup, setPopup] = useState<{ kind: "success" | "error"; message: string } | null>(null);

  const available = balance?.available_cents ?? 0;
  // PayPal withdrawals are always in USD; bank deposits follow the locale currency.
  const effectiveIsUSD = method === "paypal" || isUSD;
  const availableDisplay = effectiveIsUSD
    ? Math.round(gtqToUSD(available / 100, sellRate) * 100)
    : available;
  const currency = effectiveIsUSD ? "USD" : "GTQ";
  const kycApproved = kyc?.status === "approved";

  const { data: limits } = useQuery({
    queryKey: ["withdrawal-limits"],
    queryFn: async () => {
      const token = await getToken();
      return api.getWithdrawalLimits(token!);
    },
    staleTime: 5 * 60_000,
  });

  const minCents = effectiveIsUSD
    ? (limits?.min_usd_cents ?? 400)
    : (limits?.min_gtq_cents ?? 3000);

  const { data: banks, isLoading: loadingBanks } = useQuery({
    queryKey: ["gt-banks"],
    queryFn: async () => {
      const token = await getToken();
      return api.getBanks(token!);
    },
    enabled: kycApproved,
    staleTime: 5 * 60_000,
  });

  const { data: accountTypes, isLoading: loadingAccountTypes } = useQuery({
    queryKey: ["bank-account-types"],
    queryFn: async () => {
      const token = await getToken();
      return api.getBankAccountTypes(token!);
    },
    enabled: kycApproved,
    staleTime: 5 * 60_000,
  });

  const parsed = Number.parseFloat(amount);
  const amountCents = Number.isFinite(parsed) ? Math.round(parsed * 100) : 0;
  const belowMinimum = amountCents > 0 && amountCents < minCents;
  const exceedsBalance = amountCents > 0 && amountCents > availableDisplay;
  const valid = isWithdrawValid(method, amountCents, minCents, availableDisplay, paypalEmail, bankName, accountType, accountNumber, accountHolder);

  const mutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      const payout_details = buildPayoutDetails(method, bankName, accountType, accountNumber, accountHolder, paypalEmail);
      return api.createWithdrawal(
        token!,
        { amount_cents: amountCents, currency, method, payout_details },
        crypto.randomUUID(),
      );
    },
    onSuccess: () => {
      setPopup({ kind: "success", message: t("withdraw.successMsg") });
      setAmount("");
      setBankName("");
      setAccountType("");
      setAccountNumber("");
      setAccountHolder("");
      setPaypalEmail("");
    },
    onError: () => setPopup({ kind: "error", message: t("withdraw.errorGeneric") }),
  });

  if (kycLoading) {
    return (
      <div className="max-w-lg mx-auto space-y-4">
        <h1 className="font-display text-3xl text-white">{t("withdraw.title")}</h1>
        <div className="card p-8 flex justify-center">
          <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      </div>
    );
  }

  if (!kycApproved) {
    return (
      <div className="max-w-lg mx-auto space-y-4">
        <h1 className="font-display text-3xl text-white">{t("withdraw.title")}</h1>
        <div className="card p-8 flex flex-col items-center text-center gap-4">
          <ShieldAlert className="w-12 h-12 text-gold-400" />
          <div className="space-y-2">
            <h2 className="text-lg font-semibold text-white mb-2">
              {t("withdraw.kycRequired")}
            </h2>
            <p className="text-text-secondary text-sm">
              {t("withdraw.kycRequiredDesc")}
            </p>
            <p className="text-text-muted text-sm">
              {t("withdraw.kycRequiredAction")}
            </p>
          </div>
          <Link href="/kyc" className="btn-gold px-6 py-2 inline-block">
            {t("withdraw.kycAction")}
          </Link>
        </div>
      </div>
    );
  }

  const amountLabel = effectiveIsUSD ? formatUSD(amountCents) : formatGTQ(amountCents);
  const submitLabel = valid ? `${t("withdraw.submit")} ${amountLabel}` : t("withdraw.submit");

  return (
    <>
    <div className="max-w-lg mx-auto space-y-6">
      <h1 className="font-display text-3xl text-white">{t("withdraw.title")}</h1>

      {/* Available balance */}
      <div className="card p-4 flex items-center justify-between">
        <span className="text-sm text-text-secondary">{t("withdraw.available")}</span>
        <span className="font-score text-xl text-white">{fmt(available)}</span>
      </div>

      <div className="card p-6 space-y-5">
        {/* Method tabs */}
        <div>
          <p className="block text-sm text-text-secondary mb-2">{t("withdraw.method")}</p>
          <div className="flex gap-2">
            {(
              [
                { id: "bank_gt", label: t("withdraw.methodBankGT"), icon: <Banknote className="w-4 h-4" /> },
                { id: "paypal", label: t("withdraw.methodPayPal"),  icon: <Globe className="w-4 h-4" />    },
              ] as const
            ).map((m) => (
              <button
                key={m.id}
                type="button"
                onClick={() => { setMethod(m.id); setAmount(""); setPopup(null); }}
                className={`flex-1 flex items-center justify-center gap-1.5 py-2 px-3 rounded-lg text-sm border transition-colors ${
                  method === m.id
                    ? "bg-blue-700 border-blue-500 text-white"
                    : "border-blue-700 text-text-muted hover:text-text-secondary"
                }`}
              >
                {m.icon}
                {m.label}
              </button>
            ))}
          </div>
        </div>

        {/* Amount */}
        <div>
          <label
            htmlFor="withdraw-amount"
            className="block text-sm text-text-secondary mb-1.5"
          >
            {effectiveIsUSD ? t("withdraw.amountUSD") : t("withdraw.amount")}
          </label>
          <input
            id="withdraw-amount"
            type="number"
            min={minCents / 100}
            step="0.01"
            max={availableDisplay / 100}
            value={amount}
            onChange={(e) => { setAmount(e.target.value); setPopup(null); }}
            placeholder={(minCents / 100).toFixed(2)}
            className="input-base"
          />
          {belowMinimum && (
            <p className="text-red-400 text-xs mt-1">
              {t("withdraw.amountBelowMin")} {effectiveIsUSD ? formatUSD(minCents) : formatGTQ(minCents)}
            </p>
          )}
          {exceedsBalance && (
            <p className="text-red-400 text-xs mt-1">{t("withdraw.amountExceeds")}</p>
          )}
        </div>

        {/* Bank GT fields */}
        {method === "bank_gt" && (
          <>
            <FormField label={t("withdraw.bankSelect")} spacing="normal">
              <select
                value={bankName}
                onChange={(e) => setBankName(e.target.value)}
                className="input-base"
                disabled={loadingBanks}
              >
                <option value="">
                  {loadingBanks ? t("withdraw.bankSelectLoading") : t("withdraw.bankSelectPh")}
                </option>
                {(banks ?? []).map((b) => (
                  <option key={b.id} value={b.name}>
                    {b.name}
                  </option>
                ))}
              </select>
            </FormField>

            <FormField label={t("withdraw.accountType")} spacing="normal">
              <select
                value={accountType}
                onChange={(e) => setAccountType(e.target.value)}
                className="input-base"
                disabled={loadingAccountTypes}
              >
                <option value="">
                  {loadingAccountTypes ? t("withdraw.accountTypeLoading") : t("withdraw.accountTypePh")}
                </option>
                {(accountTypes ?? []).map((at) => (
                  <option key={at.id} value={at.name}>
                    {accountTypeName(at.name)}
                  </option>
                ))}
              </select>
            </FormField>

            <FormField label={t("withdraw.accountNumber")} spacing="normal">
              <input
                type="text"
                value={accountNumber}
                onChange={(e) => setAccountNumber(e.target.value)}
                placeholder={t("withdraw.accountNumberPh")}
                className="input-base"
              />
            </FormField>

            <FormField label={t("withdraw.accountHolder")} spacing="normal">
              <input
                type="text"
                value={accountHolder}
                onChange={(e) => setAccountHolder(e.target.value)}
                placeholder={t("withdraw.accountHolderPh")}
                className="input-base"
              />
            </FormField>
          </>
        )}

        {/* PayPal field */}
        {method === "paypal" && (
          <FormField label={t("withdraw.paypalEmail")} spacing="normal">
            <input
              id="withdraw-paypal-email"
              type="email"
              value={paypalEmail}
              onChange={(e) => setPaypalEmail(e.target.value)}
              placeholder={t("withdraw.paypalEmailPh")}
              className="input-base"
            />
          </FormField>
        )}

        <SubmitButton
          isPending={mutation.isPending}
          disabled={!valid}
          onClick={() => {
            setPopup(null);
            mutation.mutate();
          }}
        >
          {submitLabel}
        </SubmitButton>
      </div>
    </div>

    {/* Result popup */}
    {popup && (
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
        <div className="bg-surface border border-white/10 rounded-2xl shadow-2xl w-full max-w-sm p-6 flex flex-col items-center gap-4 text-center">
          {popup.kind === "success" ? (
            <CheckCircle2 className="w-14 h-14 text-green-400" />
          ) : (
            <XCircle className="w-14 h-14 text-red-400" />
          )}
          <div>
            <p className="text-white font-semibold text-lg mb-1">
              {popup.kind === "success" ? t("withdraw.successTitle") : t("withdraw.errorTitle")}
            </p>
            <p className="text-text-secondary text-sm">{popup.message}</p>
          </div>
          <div className="flex gap-3 w-full">
            {popup.kind === "error" && (
              <button
                type="button"
                onClick={() => { setPopup(null); mutation.mutate(); }}
                className="flex-1 btn-gold py-2 text-sm"
              >
                {t("withdraw.errorRetry")}
              </button>
            )}
            <button
              type="button"
              onClick={() => setPopup(null)}
              className="flex-1 flex items-center justify-center gap-1.5 border border-white/20 rounded-lg py-2 text-sm text-text-secondary hover:text-white transition-colors"
            >
              <X className="w-4 h-4" />
              {t("withdraw.errorClose")}
            </button>
          </div>
        </div>
      </div>
    )}
    </>
  );
}
