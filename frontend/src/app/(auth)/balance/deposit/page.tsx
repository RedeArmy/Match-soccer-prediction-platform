"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import { useRouter } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  PayPalScriptProvider,
  PayPalButtons,
  FUNDING,
} from "@paypal/react-paypal-js";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  CreditCard,
  Building2,
} from "lucide-react";
import { useExchangeRate } from "@/hooks/useExchangeRate";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import {
  sniffMIME,
  isAllowedUploadType,
  formatGTQ,
  usdToGTQ,
} from "@/lib/utils";
import { FileUploadField } from "@/components/shared/FileUploadField";
import { SubmitButton } from "@/components/shared/SubmitButton";

// ── Types ─────────────────────────────────────────────────────────────────────

type Method = "recurrente" | "paypal" | "bank";
type FeatureFlags = Record<string, string>;
type PayPalActions = { order?: { capture: () => Promise<unknown> } };
type FeedbackKind = "error" | "warning" | "success";
interface Feedback {
  kind: FeedbackKind;
  message: string;
}

// ── Constants ─────────────────────────────────────────────────────────────────

const PAYPAL_CLIENT_ID = process.env.NEXT_PUBLIC_PAYPAL_CLIENT_ID ?? "";

const paypalErrorKeyByCode: Record<string, string> = {
  SERVICE_UNAVAILABLE: "deposit.paypalServiceUnavailable",
  UPSTREAM_ERROR: "deposit.paypalUpstreamError",
  VALIDATION: "deposit.paypalInvalidRequest",
  UNAUTHORISED: "deposit.paypalSessionExpired",
};

const recurrenteErrorKeyByCode: Record<string, string> = {
  UNAUTHORISED: "deposit.recurrenteSessionExpired",
  VALIDATION: "deposit.recurrenteUnavailable",
  INTERNAL: "deposit.recurrenteError",
};

// ── Helpers ───────────────────────────────────────────────────────────────────

function apiErrorMessage(
  err: unknown,
  t: (key: string) => string,
  keyByCode: Record<string, string>,
  fallbackKey: string,
): string {
  const code = (err as { code?: string })?.code;
  const key = code ? keyByCode[code] : undefined;
  if (key) return t(key);
  return err instanceof Error ? err.message : t(fallbackKey);
}

function useCountry(): { country: string | null; isLoading: boolean } {
  const { data, isLoading, isError } = useQuery<{ country: string }>({
    queryKey: ["geo-country"],
    queryFn: () => fetch("/api/geo").then((r) => r.json()),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  });
  // When the geo lookup fails, default to GT so Guatemalan users are not
  // incorrectly shown non-GT payment options due to a transient network error.
  if (isError) return { country: "GT", isLoading: false };
  return { country: data?.country ?? null, isLoading };
}

function useFeatureFlags(): FeatureFlags {
  const { data } = useQuery<FeatureFlags>({
    queryKey: ["feature-flags"],
    queryFn: () => fetch("/api/feature-flags").then((r) => r.json()),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
  return data ?? {};
}

// ── DepositAlert ──────────────────────────────────────────────────────────────

const alertConfig: Record<
  FeedbackKind,
  { bg: string; border: string; text: string; icon: React.ReactNode }
> = {
  error: {
    bg: "bg-red-500/10",
    border: "border-red-500/30",
    text: "text-red-300",
    icon: <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />,
  },
  warning: {
    bg: "bg-amber-400/10",
    border: "border-amber-400/25",
    text: "text-amber-200",
    icon: <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />,
  },
  success: {
    bg: "bg-emerald-500/10",
    border: "border-emerald-500/30",
    text: "text-emerald-300",
    icon: <CheckCircle2 className="w-4 h-4 shrink-0 mt-0.5" />,
  },
};

function DepositAlert({ feedback }: Readonly<{ feedback: Feedback | null }>) {
  if (!feedback) return null;
  const { bg, border, text, icon } = alertConfig[feedback.kind];
  return (
    <div
      className={`flex items-start gap-2.5 rounded-xl border px-4 py-3 text-sm ${bg} ${border} ${text}`}
      role={feedback.kind === "success" ? "status" : "alert"}
    >
      {icon}
      <span>{feedback.message}</span>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function DepositPage() {
  const { getToken } = useAuth();
  const { data: rate } = useExchangeRate();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const router = useRouter();

  const { country, isLoading: geoLoading } = useCountry();
  const isGuatemala = country === "GT";
  const flags = useFeatureFlags();
  const paypalEnabled = flags["paypal"] === "1";

  const [method, setMethod] = useState<Method>("recurrente");
  const [amountGTQ, setAmountGTQ] = useState("");
  const [amountUSD, setAmountUSD] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState("");
  const [feedback, setFeedback] = useState<Feedback | null>(null);

  const clearFeedback = () => setFeedback(null);

  // When geo or flags resolve, drop the active tab if it is no longer available.
  // `method` is included so a stale closure never misses a pending reset when
  // the user switches tabs just as geo data arrives. `setMethod` is a stable
  // React dispatcher and is intentionally omitted per the rules-of-hooks spec.
  useEffect(() => {
    if (geoLoading) return;
    if (isGuatemala && method === "paypal") setMethod("recurrente");
    if (!isGuatemala && method === "bank") setMethod("recurrente");
    if (!paypalEnabled && method === "paypal") setMethod("recurrente");
  }, [geoLoading, isGuatemala, paypalEnabled, method]);

  const gtqEquiv =
    rate && amountUSD
      ? formatGTQ(
          Math.round(
            usdToGTQ(Number.parseFloat(amountUSD), rate.buy_rate) * 100,
          ),
        )
      : null;

  // ── Mutations ──────────────────────────────────────────────────────────────

  const recurrenteMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      if (isGuatemala) {
        const cents = Math.round(Number.parseFloat(amountGTQ) * 100);
        return api.createPaymentIntent(
          token!,
          { amount_cents: cents, currency: "GTQ", provider: "recurrente" },
          crypto.randomUUID(),
        );
      }
      const cents = Math.round(Number.parseFloat(amountUSD) * 100);
      return api.createPaymentIntent(
        token!,
        { amount_cents: cents, currency: "USD", provider: "recurrente" },
        crypto.randomUUID(),
      );
    },
    onSuccess: (data) => {
      if (data.redirect_url) globalThis.location.href = data.redirect_url;
    },
    onError: (e: Error) =>
      setFeedback({
        kind: "error",
        message: apiErrorMessage(
          e,
          t,
          recurrenteErrorKeyByCode,
          "deposit.recurrenteError",
        ),
      }),
  });

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error(t("deposit.bankFileNoAttach"));
      const token = await getToken();
      const fd = new FormData();
      fd.append(
        "amount_cents",
        String(Math.round(Number.parseFloat(amountGTQ) * 100)),
      );
      fd.append("currency", "GTQ");
      fd.append("file", file);
      return api.uploadBankTransfer(token!, fd, crypto.randomUUID());
    },
    onSuccess: () => {
      setFeedback({ kind: "success", message: t("deposit.bankSuccess") });
      setFile(null);
      setAmountGTQ("");
    },
    onError: (e: Error) => setFeedback({ kind: "error", message: e.message }),
  });

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    setFileError("");
    const f = e.target.files?.[0];
    if (!f) return;
    if (f.size > 5 * 1024 * 1024) {
      setFileError(t("deposit.bankFileTooBig"));
      return;
    }
    const mime = await sniffMIME(f);
    if (!isAllowedUploadType(mime)) {
      setFileError(t("deposit.bankFileInvalidType"));
      return;
    }
    setFile(f);
  }

  const validUSD = amountUSD && Number.parseFloat(amountUSD) > 0;
  const paypalAmountCents = validUSD
    ? Math.round(Number.parseFloat(amountUSD) * 100)
    : 0;

  // ── Tab definitions ────────────────────────────────────────────────────────

  type TabDef = { id: Method; label: string; icon: React.ReactNode };

  const allTabs: TabDef[] = [
    {
      id: "recurrente",
      label: t("deposit.tabRecurrente"),
      icon: <CreditCard className="w-4 h-4" />,
    },
    {
      id: "paypal",
      label: t("deposit.tabPaypal"),
      icon: <CreditCard className="w-4 h-4" />,
    },
    {
      id: "bank",
      label: t("deposit.tabBank"),
      icon: <Building2 className="w-4 h-4" />,
    },
  ];

  let visibleTabs: TabDef[];
  if (geoLoading) {
    visibleTabs = allTabs.filter((tab) => tab.id === "recurrente");
  } else if (isGuatemala) {
    visibleTabs = allTabs.filter((tab) => tab.id !== "paypal");
  } else {
    visibleTabs = allTabs.filter(
      (tab) => tab.id !== "bank" && (tab.id !== "paypal" || paypalEnabled),
    );
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="max-w-lg mx-auto space-y-6">
      <h1 className="font-display text-3xl text-white">DEPOSITAR</h1>

      {/* Method tabs */}
      <div className="flex gap-1 p-1 bg-blue-900 rounded-xl">
        {geoLoading
          ? (["sk-tab-1", "sk-tab-2"] as const).map((id) => (
              <div
                key={id}
                className="flex-1 h-9 rounded-lg animate-pulse bg-blue-800/60"
              />
            ))
          : visibleTabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                onClick={() => {
                  setMethod(tab.id);
                  clearFeedback();
                }}
                className={`flex-1 flex items-center justify-center gap-1.5 py-2 px-3 rounded-lg text-sm transition-colors ${
                  method === tab.id
                    ? "bg-blue-700 text-white font-medium"
                    : "text-text-muted hover:text-text-secondary"
                }`}
              >
                {tab.icon}
                {tab.label}
              </button>
            ))}
      </div>

      <div className="card p-6 space-y-5">
        {/* ── Recurrente ── */}
        {method === "recurrente" && (
          <>
            <p className="text-sm text-text-secondary">
              {t("deposit.recurrenteDesc")}
            </p>
            {isGuatemala ? (
              <div>
                <label
                  htmlFor="deposit-gtq-amount"
                  className="block text-sm text-text-secondary mb-1.5"
                >
                  {t("deposit.amountGTQLabel")}
                </label>
                <input
                  id="deposit-gtq-amount"
                  type="number"
                  min="10"
                  max="100000"
                  step="0.01"
                  value={amountGTQ}
                  onChange={(e) => {
                    setAmountGTQ(e.target.value);
                    clearFeedback();
                  }}
                  placeholder="100.00"
                  className="input-base"
                />
              </div>
            ) : (
              <div>
                <label
                  htmlFor="deposit-usd-amount-recurrente"
                  className="block text-sm text-text-secondary mb-1.5"
                >
                  {t("deposit.amountUSDLabel")}
                </label>
                <input
                  id="deposit-usd-amount-recurrente"
                  type="number"
                  min="1"
                  max="10000"
                  step="0.01"
                  value={amountUSD}
                  onChange={(e) => {
                    setAmountUSD(e.target.value);
                    clearFeedback();
                  }}
                  placeholder="50.00"
                  className="input-base"
                />
                {gtqEquiv && (
                  <p className="text-xs text-text-muted mt-1.5">
                    ≈ {gtqEquiv} GTQ
                  </p>
                )}
              </div>
            )}
            <SubmitButton
              isPending={recurrenteMutation.isPending}
              disabled={isGuatemala ? !amountGTQ : !amountUSD}
              onClick={() => recurrenteMutation.mutate()}
            >
              {t("deposit.recurrenteContinue")}
            </SubmitButton>
          </>
        )}

        {/* ── PayPal — only for users outside Guatemala ── */}
        {method === "paypal" && !isGuatemala && (
          <>
            <p className="text-sm text-text-secondary">
              {t("deposit.paypalDesc")}
            </p>
            {rate && (
              <p className="text-xs text-text-muted">
                {t("deposit.buyRateLabel")}{" "}
                <span className="text-gold-400 font-medium">
                  Q{Number.parseFloat(rate.buy_rate).toFixed(4)} / USD
                </span>
              </p>
            )}
            <div>
              <label
                htmlFor="deposit-usd-amount"
                className="block text-sm text-text-secondary mb-1.5"
              >
                {t("deposit.amountUSDLabel")}
              </label>
              <input
                id="deposit-usd-amount"
                type="number"
                min="1"
                max="10000"
                step="0.01"
                value={amountUSD}
                onChange={(e) => {
                  setAmountUSD(e.target.value);
                  clearFeedback();
                }}
                placeholder="50.00"
                className="input-base"
              />
              {gtqEquiv && (
                <p className="text-xs text-text-muted mt-1.5">
                  ≈ {gtqEquiv} GTQ
                </p>
              )}
            </div>

            {!PAYPAL_CLIENT_ID && (
              <div className="flex items-start gap-2.5 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">
                <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
                <span>
                  {t("deposit.paypalNotConfiguredPre")}{" "}
                  <code className="font-mono text-xs">
                    NEXT_PUBLIC_PAYPAL_CLIENT_ID
                  </code>{" "}
                  {t("deposit.paypalNotConfiguredPost")}
                </span>
              </div>
            )}

            {PAYPAL_CLIENT_ID && validUSD && (
              <PayPalScriptProvider
                options={{
                  clientId: PAYPAL_CLIENT_ID,
                  currency: "USD",
                  intent: "capture",
                  disableFunding: "card,credit,venmo,paylater",
                }}
              >
                <PayPalButtons
                  key={String(paypalAmountCents)}
                  fundingSource={FUNDING.PAYPAL}
                  style={{
                    color: "gold",
                    shape: "rect",
                    label: "paypal",
                    height: 44,
                  }}
                  createOrder={async () => {
                    const token = await getToken();
                    if (!token)
                      throw new Error(t("deposit.paypalSessionExpired"));
                    const order = await api.createPayPalOrder(token, {
                      amount_cents: paypalAmountCents,
                      currency: "USD",
                    });
                    if (!order.id)
                      throw new Error(t("deposit.paypalNoOrderId"));
                    return order.id;
                  }}
                  onApprove={async (_data: unknown, actions: PayPalActions) => {
                    await actions.order!.capture();
                    await Promise.all([
                      queryClient.invalidateQueries({ queryKey: ["balance"] }),
                      queryClient.invalidateQueries({ queryKey: ["ledger"] }),
                      queryClient.invalidateQueries({
                        queryKey: ["ledger-preview"],
                      }),
                    ]);
                    const usd = (paypalAmountCents / 100).toFixed(2);
                    router.replace(`/balance?paypal=success&usd=${usd}`);
                  }}
                  onCancel={() =>
                    setFeedback({
                      kind: "warning",
                      message: t("deposit.paypalCancelled"),
                    })
                  }
                  onError={(err: unknown) => {
                    console.error("[PayPal] onError:", err);
                    setFeedback({
                      kind: "error",
                      message: apiErrorMessage(
                        err,
                        t,
                        paypalErrorKeyByCode,
                        "deposit.paypalError",
                      ),
                    });
                  }}
                />
              </PayPalScriptProvider>
            )}
          </>
        )}

        {/* ── Bank transfer — only for users inside Guatemala ── */}
        {method === "bank" && isGuatemala && (
          <>
            <div className="bg-blue-900/80 rounded-2xl border border-blue-700/50 p-3 space-y-3 text-sm">
              <div className="overflow-hidden rounded-xl border border-blue-700/60 bg-blue-950">
                <Image
                  src="/images/bank-transfer-info.svg"
                  alt={t("deposit.bankImageAlt")}
                  width={960}
                  height={640}
                  className="w-full h-auto"
                  priority={false}
                />
              </div>
              <p className="rounded-xl border border-amber-400/20 bg-amber-400/10 px-3 py-2 text-xs font-medium text-amber-200">
                {t("deposit.bankNoteGT")}
              </p>
              <p className="text-text-muted text-xs">{t("deposit.bankHelp")}</p>
            </div>

            <div>
              <label
                htmlFor="bank-amount"
                className="block text-sm text-text-secondary mb-1.5"
              >
                {t("deposit.bankAmountLabel")}
              </label>
              <input
                id="bank-amount"
                type="number"
                min="10"
                max="1000000"
                step="0.01"
                value={amountGTQ}
                onChange={(e) => {
                  setAmountGTQ(e.target.value);
                  clearFeedback();
                }}
                placeholder="500.00"
                className="input-base"
              />
            </div>

            <div>
              <p className="block text-sm text-text-secondary mb-1.5">
                {t("deposit.bankFileLabel")}
              </p>
              <FileUploadField
                label={t("deposit.bankFileButton")}
                fileName={file?.name}
                onChange={handleFileChange}
              />
              {fileError && (
                <div className="flex items-center gap-2 mt-2 text-xs text-red-300">
                  <AlertCircle className="w-3.5 h-3.5 shrink-0" />
                  {fileError}
                </div>
              )}
            </div>

            <SubmitButton
              isPending={uploadMutation.isPending}
              disabled={!file || !amountGTQ}
              onClick={() => uploadMutation.mutate()}
            >
              {t("deposit.bankSubmit")}
            </SubmitButton>
          </>
        )}

        {/* ── Inline feedback alert ── */}
        <DepositAlert feedback={feedback} />
      </div>
    </div>
  );
}
