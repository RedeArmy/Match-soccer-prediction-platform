"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useBalance } from "@/hooks/useBalance";
import { useKYCStatus } from "@/hooks/useKYCStatus";
import { formatGTQ } from "@/lib/utils";
import { FormFeedback } from "@/components/shared/FormFeedback";
import { SubmitButton } from "@/components/shared/SubmitButton";
import { ShieldAlert, Banknote, Globe } from "lucide-react";
import Link from "next/link";

type Method = "bank_gt" | "paypal";

export default function WithdrawPage() {
  const { getToken } = useAuth();
  const { data: balance } = useBalance();
  const { data: kyc } = useKYCStatus();
  const [method, setMethod] = useState<Method>("bank_gt");
  const [amount, setAmount] = useState("");
  const [bankName, setBankName] = useState("");
  const [accountNumber, setAccountNumber] = useState("");
  const [accountHolder, setAccountHolder] = useState("");
  const [paypalEmail, setPaypalEmail] = useState("");
  const [success, setSuccess] = useState("");
  const [error, setError] = useState("");

  const available = balance?.available_cents ?? 0;
  const kycApproved = kyc?.status === "approved";
  const amountCents = Math.round(Number.parseFloat(amount) * 100);
  const valid = amountCents > 0 && amountCents <= available;

  const mutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      const extra: Record<string, unknown> =
        method === "bank_gt"
          ? {
              bank_name: bankName,
              account_number: accountNumber,
              account_holder: accountHolder,
            }
          : { paypal_email: paypalEmail };
      return api.createWithdrawal(
        token!,
        { amount_cents: amountCents, currency: "GTQ", method, ...extra },
        crypto.randomUUID(),
      );
    },
    onSuccess: () => {
      setSuccess(
        "Solicitud de retiro enviada. Será procesada en 1-3 días hábiles.",
      );
      setAmount("");
    },
    onError: (e: Error) => setError(e.message),
  });

  if (!kycApproved) {
    return (
      <div className="max-w-lg mx-auto space-y-4">
        <h1 className="font-display text-3xl text-white">RETIRAR</h1>
        <div className="card p-8 flex flex-col items-center text-center gap-4">
          <ShieldAlert className="w-12 h-12 text-gold-400" />
          <div>
            <h2 className="text-lg font-semibold text-white mb-2">
              Verificación requerida
            </h2>
            <p className="text-text-secondary text-sm">
              Debes completar tu verificación KYC (Tier 2) para poder retirar
              fondos.
            </p>
          </div>
          <Link href="/kyc" className="btn-gold px-6 py-2 inline-block">
            Completar verificación
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-lg mx-auto space-y-6">
      <h1 className="font-display text-3xl text-white">RETIRAR</h1>

      {/* Available balance */}
      <div className="card p-4 flex items-center justify-between">
        <span className="text-sm text-text-secondary">
          Disponible para retirar
        </span>
        <span className="font-score text-xl text-white">
          {formatGTQ(available)}
        </span>
      </div>

      <div className="card p-6 space-y-5">
        {/* Method */}
        <div>
          <p className="block text-sm text-text-secondary mb-2">
            Método de retiro
          </p>
          <div className="flex gap-2">
            {(
              [
                {
                  id: "bank_gt",
                  label: "Banco GT",
                  icon: <Banknote className="w-4 h-4" />,
                },
                {
                  id: "paypal",
                  label: "PayPal",
                  icon: <Globe className="w-4 h-4" />,
                },
              ] as const
            ).map((m) => (
              <button
                key={m.id}
                onClick={() => setMethod(m.id)}
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
            Monto a retirar (GTQ)
          </label>
          <input
            id="withdraw-amount"
            type="number"
            min="50"
            step="0.01"
            max={available / 100}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="100.00"
            className="input-base"
          />
          {amountCents > available && (
            <p className="text-red-400 text-xs mt-1">
              Excede el balance disponible
            </p>
          )}
        </div>

        {/* Bank fields */}
        {method === "bank_gt" && (
          <>
            {[
              {
                value: bankName,
                setter: setBankName,
                label: "Nombre del banco",
                placeholder: "Ej. Banco Industrial",
              },
              {
                value: accountNumber,
                setter: setAccountNumber,
                label: "Número de cuenta",
                placeholder: "000-000000-0",
              },
              {
                value: accountHolder,
                setter: setAccountHolder,
                label: "Titular de la cuenta",
                placeholder: "Nombre completo",
              },
            ].map(({ value, setter, label, placeholder }) => (
              <div key={label}>
                <label className="block text-sm text-text-secondary mb-1.5">
                  {label}
                </label>
                <input
                  type="text"
                  value={value}
                  onChange={(e) => setter(e.target.value)}
                  placeholder={placeholder}
                  className="input-base"
                />
              </div>
            ))}
          </>
        )}

        {/* PayPal field */}
        {method === "paypal" && (
          <div>
            <label
              htmlFor="withdraw-paypal-email"
              className="block text-sm text-text-secondary mb-1.5"
            >
              Correo PayPal
            </label>
            <input
              id="withdraw-paypal-email"
              type="email"
              value={paypalEmail}
              onChange={(e) => setPaypalEmail(e.target.value)}
              placeholder="tu@email.com"
              className="input-base"
            />
          </div>
        )}

        <SubmitButton
          isPending={mutation.isPending}
          disabled={!valid}
          onClick={() => {
            setError("");
            mutation.mutate();
          }}
        >
          {`Retirar ${formatGTQ(amountCents)}`}
        </SubmitButton>

        <FormFeedback error={error} success={success} center />
      </div>
    </div>
  );
}
