"use client";

import { useEffect, useRef, useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation } from "@tanstack/react-query";
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

type PayPalOrderActions = {
  order: {
    create: (order: {
      intent: "CAPTURE";
      purchase_units: Array<{
        custom_id: string;
        amount: {
          currency_code: "USD";
          value: string;
        };
      }>;
    }) => Promise<string>;
    capture: () => Promise<unknown>;
  };
};

type PayPalButtonsInstance = {
  render: (container: HTMLElement) => Promise<void>;
  close?: () => void;
};

type PayPalNamespace = {
  Buttons: (options: {
    style?: Record<string, string>;
    createOrder: (_data: unknown, actions: PayPalOrderActions) => Promise<string>;
    onApprove: (_data: unknown, actions: PayPalOrderActions) => Promise<void>;
    onError: (error: unknown) => void;
  }) => PayPalButtonsInstance;
};

declare global {
  interface Window {
    paypal?: PayPalNamespace;
  }
}

const paypalClientId = process.env.NEXT_PUBLIC_PAYPAL_CLIENT_ID ?? "";
const paypalScriptId = "paypal-js-sdk";

export default function DepositPage() {
  const { getToken } = useAuth();
  const { data: rate } = useExchangeRate();
  const [method, setMethod] = useState<Method>("recurrente");
  const [amountGTQ, setAmountGTQ] = useState("");
  const [amountUSD, setAmountUSD] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState("");
  const [success, setSuccess] = useState("");
  const [error, setError] = useState("");

  const gtqEquiv =
    rate && amountUSD
      ? formatGTQ(
          Math.round(
            usdToGTQ(Number.parseFloat(amountUSD), rate.buy_rate) * 100,
          ),
        )
      : null;

  // Recurrente / PayPal: create payment intent
  const intentMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      const isUSD = method === "paypal";
      const rawAmount = isUSD
        ? Number.parseFloat(amountUSD)
        : Number.parseFloat(amountGTQ);
      const cents = Math.round(rawAmount * 100);
      return api.createPaymentIntent(
        token!,
        {
          amount_cents: cents,
          currency: isUSD ? "USD" : "GTQ",
          provider: method,
        },
        crypto.randomUUID(),
      );
    },
    onSuccess: (data) => {
      if (data.redirect_url) {
        globalThis.location.href = data.redirect_url;
        return;
      }
      setError("El proveedor de pago no devolvió una URL de redirección.");
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

  return (
    <div className="max-w-lg mx-auto space-y-6">
      <h1 className="font-display text-3xl text-white">DEPOSITAR</h1>

      {/* Method tabs */}
      <div className="flex gap-1 p-1 bg-blue-900 rounded-xl">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => {
              setMethod(t.id);
              setError("");
              setSuccess("");
            }}
            className={`flex-1 flex items-center justify-center gap-1.5 py-2 px-3 rounded-lg text-sm transition-colors ${
              method === t.id
                ? "bg-blue-700 text-white font-medium"
                : "text-text-muted hover:text-text-secondary"
            }`}
          >
            {t.icon}
            {t.label}
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
              isPending={intentMutation.isPending}
              disabled={!amountGTQ}
              onClick={() => intentMutation.mutate()}
            >
              Continuar con Recurrente
            </SubmitButton>
          </>
        )}

        {/* PayPal */}
        {method === "paypal" && (
          <>
            <p className="text-sm text-text-secondary">
              Deposita con PayPal. Se usa la tasa de compra del día.
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
                onChange={(e) => setAmountUSD(e.target.value)}
                placeholder="50.00"
                className="input-base"
              />
              {gtqEquiv && (
                <p className="text-xs text-text-muted mt-1">≈ {gtqEquiv} GTQ</p>
              )}
            </div>
            <PayPalCheckoutButton
              amountUSD={amountUSD}
              getToken={getToken}
              onSuccess={() => {
                setSuccess("Pago PayPal completado. El balance se actualizará cuando PayPal confirme la captura.");
                setAmountUSD("");
              }}
              onError={(message) => setError(message)}
            />
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

function PayPalCheckoutButton({
  amountUSD,
  getToken,
  onSuccess,
  onError,
}: Readonly<{
  amountUSD: string;
  getToken: () => Promise<string | null>;
  onSuccess: () => void;
  onError: (message: string) => void;
}>) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const buttonsRef = useRef<PayPalButtonsInstance | null>(null);
  const [sdkReady, setSdkReady] = useState(false);
  const [sdkError, setSdkError] = useState("");

  const amount = Number.parseFloat(amountUSD);
  const amountReady = Number.isFinite(amount) && amount > 0;

  useEffect(() => {
    if (!paypalClientId) {
      setSdkError("Configura NEXT_PUBLIC_PAYPAL_CLIENT_ID para habilitar PayPal.");
      return;
    }

    if (globalThis.window?.paypal) {
      setSdkReady(true);
      return;
    }

    const existing = document.getElementById(paypalScriptId) as HTMLScriptElement | null;
    if (existing) {
      existing.addEventListener("load", () => setSdkReady(true), { once: true });
      existing.addEventListener("error", () => setSdkError("No se pudo cargar el SDK de PayPal."), { once: true });
      return;
    }

    const script = document.createElement("script");
    script.id = paypalScriptId;
    script.src = `https://www.paypal.com/sdk/js?client-id=${encodeURIComponent(paypalClientId)}&currency=USD&intent=capture&components=buttons`;
    script.async = true;
    script.onload = () => setSdkReady(true);
    script.onerror = () => setSdkError("No se pudo cargar el SDK de PayPal.");
    document.body.appendChild(script);
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || !sdkReady || !globalThis.window?.paypal) return;

    buttonsRef.current?.close?.();
    container.innerHTML = "";

    const buttons = globalThis.window.paypal.Buttons({
      style: {
        layout: "vertical",
        color: "gold",
        shape: "rect",
        label: "paypal",
      },
      createOrder: async (_data, actions) => {
        if (!amountReady) {
          throw new Error("Ingresa un monto USD válido antes de continuar con PayPal.");
        }

        const token = await getToken();
        if (!token) throw new Error("Sesión expirada. Inicia sesión nuevamente.");

        const intent = await api.createPaymentIntent(
          token,
          {
            amount_cents: Math.round(amount * 100),
            currency: "USD",
            provider: "paypal",
          },
          crypto.randomUUID(),
        );

        if (!intent.token) {
          throw new Error("El backend no devolvió el token PayPal requerido.");
        }

        return actions.order.create({
          intent: "CAPTURE",
          purchase_units: [
            {
              custom_id: intent.token,
              amount: {
                currency_code: "USD",
                value: amount.toFixed(2),
              },
            },
          ],
        });
      },
      onApprove: async (_data, actions) => {
        await actions.order.capture();
        onSuccess();
      },
      onError: (error) => {
        const message = error instanceof Error ? error.message : "No se pudo iniciar el checkout de PayPal.";
        onError(message);
      },
    });

    buttonsRef.current = buttons;
    void buttons.render(container).catch((error: unknown) => {
      const message = error instanceof Error ? error.message : "No se pudo renderizar el botón de PayPal.";
      setSdkError(message);
    });

    return () => {
      buttons.close?.();
      container.innerHTML = "";
    };
  }, [amount, amountReady, getToken, onError, onSuccess, sdkReady]);

  if (sdkError) {
    return (
      <p className="rounded border border-red-400/25 bg-red-400/10 px-3 py-2 text-xs text-red-200">
        {sdkError}
      </p>
    );
  }

  return (
    <div className={!amountReady ? "pointer-events-none opacity-50" : undefined}>
      {!amountReady && (
        <p className="mb-2 text-xs text-text-muted">
          Ingresa un monto USD válido para habilitar PayPal.
        </p>
      )}
      <div ref={containerRef} />
    </div>
  );
}
