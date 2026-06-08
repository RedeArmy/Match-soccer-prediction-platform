"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { PayPalScriptProvider, PayPalButtons, FUNDING } from "@paypal/react-paypal-js";
import { useExchangeRate } from "@/hooks/useExchangeRate";
import { api } from "@/lib/api";
import {
  sniffMIME,
  isAllowedUploadType,
  formatGTQ,
  usdToGTQ,
} from "@/lib/utils";
import { ImagePlaceholder } from "@/components/shared/ImagePlaceholder";
import { FormFeedback } from "@/components/shared/FormFeedback";
import { FileUploadField } from "@/components/shared/FileUploadField";
import { SubmitButton } from "@/components/shared/SubmitButton";
import { CreditCard, Building2 } from "lucide-react";

type Method = "recurrente" | "paypal" | "bank";
type PayPalActions = { order?: { capture: () => Promise<unknown> } };

const PAYPAL_CLIENT_ID = process.env.NEXT_PUBLIC_PAYPAL_CLIENT_ID ?? "";

export default function DepositPage() {
  const { getToken } = useAuth();
  const { data: rate } = useExchangeRate();
  const queryClient = useQueryClient();
  const [method, setMethod] = useState<Method>("recurrente");
  const [amountGTQ, setAmountGTQ] = useState("");
  const [amountUSD, setAmountUSD] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState("");
  const [success, setSuccess] = useState("");
  const [error, setError] = useState("");
  const [paypalOrderId, setPaypalOrderId] = useState("");
  const [paypalOrderAmountCents, setPaypalOrderAmountCents] = useState<number | null>(null);

  const gtqEquiv =
    rate && amountUSD
      ? formatGTQ(
          Math.round(
            usdToGTQ(Number.parseFloat(amountUSD), rate.buy_rate) * 100,
          ),
        )
      : null;

  // Recurrente: creates a hosted checkout session and redirects
  const recurrenteMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      const cents = Math.round(Number.parseFloat(amountGTQ) * 100);
      return api.createPaymentIntent(
        token!,
        { amount_cents: cents, currency: "GTQ", provider: "recurrente" },
        crypto.randomUUID(),
      );
    },
    onSuccess: (data) => {
      if (data.redirect_url) globalThis.location.href = data.redirect_url;
    },
    onError: (e: Error) => setError(e.message),
  });

  // Bank transfer upload
  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error("Adjunta el comprobante");
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
      setSuccess("Comprobante enviado. Será revisado en 24-48 horas.");
      setFile(null);
      setAmountGTQ("");
    },
    onError: (e: Error) => setError(e.message),
  });

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    setFileError("");
    const f = e.target.files?.[0];
    if (!f) return;
    if (f.size > 5 * 1024 * 1024) {
      setFileError("El archivo no debe superar 5 MB");
      return;
    }
    const mime = await sniffMIME(f);
    if (!isAllowedUploadType(mime)) {
      setFileError("Tipo no permitido. Usa JPEG, PNG, WebP o PDF.");
      return;
    }
    setFile(f);
  }

  const tabs: { id: Method; label: string; icon: React.ReactNode }[] = [
    {
      id: "recurrente",
      label: "Recurrente",
      icon: <CreditCard className="w-4 h-4" />,
    },
    { id: "paypal", label: "PayPal", icon: <CreditCard className="w-4 h-4" /> },
    {
      id: "bank",
      label: "Transferencia",
      icon: <Building2 className="w-4 h-4" />,
    },
  ];

  const validUSD = amountUSD && Number.parseFloat(amountUSD) > 0;
  const paypalAmountCents = validUSD ? Math.round(Number.parseFloat(amountUSD) * 100) : 0;
  const paypalOrderReady = Boolean(
    paypalOrderId && paypalOrderAmountCents === paypalAmountCents,
  );

  const preparePayPalOrderMutation = useMutation({
    mutationFn: async () => {
      if (!paypalAmountCents) throw new Error("Ingresa un monto válido en USD.");
      const token = await getToken();
      if (!token) {
        throw new Error("Sesión expirada. Inicia sesión nuevamente antes de pagar con PayPal.");
      }

      const order = await api.createPayPalOrder(token, {
        amount_cents: paypalAmountCents,
        currency: "USD",
      });

      if (!order.id) throw new Error("PayPal no devolvió un ID de orden válido.");
      return { id: order.id, amountCents: paypalAmountCents };
    },
    onSuccess: ({ id, amountCents }) => {
      setPaypalOrderId(id);
      setPaypalOrderAmountCents(amountCents);
      setError("");
    },
    onError: (e: Error) => {
      setPaypalOrderId("");
      setPaypalOrderAmountCents(null);
      setError(e.message);
    },
  });

  return (
    <div className="max-w-lg mx-auto space-y-6">
      <h1 className="font-display text-3xl text-white">DEPOSITAR</h1>

      {/* Method tabs */}
      <div className="flex gap-1 p-1 bg-blue-900 rounded-xl">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => {
              setMethod(tab.id);
              setError("");
              setSuccess("");
              setPaypalOrderId("");
              setPaypalOrderAmountCents(null);
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
        {/* Recurrente */}
        {method === "recurrente" && (
          <>
            <p className="text-sm text-text-secondary">
              Deposita con tarjeta de débito/crédito guatemalteca vía
              Recurrente. Serás redirigido al portal de pago.
            </p>
            <div>
              <label
                htmlFor="deposit-gtq-amount"
                className="block text-sm text-text-secondary mb-1.5"
              >
                Monto (GTQ)
              </label>
              <input
                id="deposit-gtq-amount"
                type="number"
                min="10"
                max="100000"
                step="0.01"
                value={amountGTQ}
                onChange={(e) => setAmountGTQ(e.target.value)}
                placeholder="100.00"
                className="input-base"
              />
            </div>
            <SubmitButton
              isPending={recurrenteMutation.isPending}
              disabled={!amountGTQ}
              onClick={() => recurrenteMutation.mutate()}
            >
              Continuar con Recurrente
            </SubmitButton>
          </>
        )}

        {/* PayPal */}
        {method === "paypal" && (
          <>
            <p className="text-sm text-text-secondary">
              Deposita con tu cuenta PayPal. Se usa la tasa de compra del día.
            </p>
            {rate && (
              <p className="text-xs text-text-muted">
                Tasa compra:{" "}
                <span className="text-gold-400">
                  Q{Number.parseFloat(rate.buy_rate).toFixed(4)} / USD
                </span>
              </p>
            )}

            <div>
              <label
                htmlFor="deposit-usd-amount"
                className="block text-sm text-text-secondary mb-1.5"
              >
                Monto (USD)
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
                  setError("");
                  setPaypalOrderId("");
                  setPaypalOrderAmountCents(null);
                }}
                placeholder="50.00"
                className="input-base"
              />
              {gtqEquiv && (
                <p className="text-xs text-text-muted mt-1">
                  ≈ {gtqEquiv} GTQ
                </p>
              )}
            </div>

            {!PAYPAL_CLIENT_ID && (
              <p className="text-xs text-red-400 text-center py-2">
                PayPal no está configurado. Agrega{" "}
                <code className="font-mono">NEXT_PUBLIC_PAYPAL_CLIENT_ID</code>{" "}
                al entorno.
              </p>
            )}

            {PAYPAL_CLIENT_ID && validUSD && !paypalOrderReady && (
              <SubmitButton
                isPending={preparePayPalOrderMutation.isPending}
                disabled={!validUSD}
                onClick={() => preparePayPalOrderMutation.mutate()}
              >
                Preparar pago con PayPal
              </SubmitButton>
            )}

            {PAYPAL_CLIENT_ID && paypalOrderReady && (
              <PayPalScriptProvider
                key={paypalOrderId}
                options={{
                  clientId: PAYPAL_CLIENT_ID,
                  currency: "USD",
                  intent: "capture",
                  disableFunding: "card,credit,venmo,paylater",
                }}
              >
                <PayPalButtons
                  fundingSource={FUNDING.PAYPAL}
                  style={{
                    color: "gold",
                    shape: "rect",
                    label: "paypal",
                    height: 44,
                  }}
                  createOrder={async () => paypalOrderId}
                  onApprove={async (_data: unknown, actions: PayPalActions) => {
                    await actions.order!.capture();
                    await Promise.all([
                      queryClient.invalidateQueries({ queryKey: ["balance"] }),
                      queryClient.invalidateQueries({ queryKey: ["ledger"] }),
                      queryClient.invalidateQueries({ queryKey: ["ledger-preview"] }),
                    ]);
                    setSuccess("Pago completado. Tu saldo será acreditado en breve.");
                    setAmountUSD("");
                    setPaypalOrderId("");
                    setPaypalOrderAmountCents(null);
                  }}
                  onCancel={() => {
                    setError("Pago cancelado.");
                    setPaypalOrderId("");
                    setPaypalOrderAmountCents(null);
                  }}
                  onError={(err: unknown) => {
                    console.error("[PayPal] onError:", err);
                    const message = err instanceof Error ? err.message : "Error al procesar el pago con PayPal.";
                    setError(message);
                  }}
                />
              </PayPalScriptProvider>
            )}
          </>
        )}

        {/* Bank transfer */}
        {method === "bank" && (
          <>
            <div className="bg-blue-900 rounded-xl p-4 space-y-2 text-sm">
              <p className="font-medium text-text-primary">Datos bancarios</p>
              <ImagePlaceholder
                aspectRatio="4/3"
                label="Bank account QR code"
                dataSrc="/images/bank-qr.png"
                className="max-w-[160px]"
              />
              <p className="text-text-muted text-xs mt-2">
                Deposita al banco y adjunta el comprobante. La revisión toma
                24-48 horas.
              </p>
            </div>

            <div>
              <label
                htmlFor="bank-amount"
                className="block text-sm text-text-secondary mb-1.5"
              >
                Monto depositado (GTQ)
              </label>
              <input
                id="bank-amount"
                type="number"
                min="10"
                max="1000000"
                step="0.01"
                value={amountGTQ}
                onChange={(e) => setAmountGTQ(e.target.value)}
                placeholder="500.00"
                className="input-base"
              />
            </div>

            <div>
              <p className="block text-sm text-text-secondary mb-1.5">
                Comprobante (JPEG, PNG, WebP, PDF — máx. 5 MB)
              </p>
              <FileUploadField
                label="Toca para adjuntar"
                fileName={file?.name}
                onChange={handleFileChange}
              />
              {fileError && (
                <p className="text-red-400 text-xs mt-1">{fileError}</p>
              )}
            </div>

            <SubmitButton
              isPending={uploadMutation.isPending}
              disabled={!file || !amountGTQ}
              onClick={() => uploadMutation.mutate()}
            >
              Enviar comprobante
            </SubmitButton>
          </>
        )}

        <FormFeedback error={error} success={success} center />
      </div>
    </div>
  );
}
