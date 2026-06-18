"use client";

import Link from "next/link";
import { ArrowLeft, CheckCircle } from "lucide-react";
import type { ReactNode } from "react";

export function DepositLoadingSpinner() {
  return (
    <div className="flex items-center justify-center py-20">
      <div className="w-6 h-6 border-2 border-gold-400 border-t-transparent rounded-full animate-spin" />
    </div>
  );
}

interface DepositNotFoundProps {
  readonly message: string;
  readonly backLabel: string;
}

export function DepositNotFound({ message, backLabel }: DepositNotFoundProps) {
  return (
    <div className="space-y-4 text-center py-16">
      <p className="text-text-muted">{message}</p>
      <Link
        href="/balance"
        className="inline-flex items-center gap-2 text-sm text-gold-400 hover:text-gold-300"
      >
        <ArrowLeft className="w-4 h-4" />
        {backLabel}
      </Link>
    </div>
  );
}

interface DepositSuccessProps {
  readonly title: string;
  readonly desc: string;
  readonly backLabel: string;
}

export function DepositSuccess({ title, desc, backLabel }: DepositSuccessProps) {
  return (
    <div className="space-y-6 max-w-md mx-auto">
      <div className="flex items-start gap-3 p-5 bg-emerald-900/40 border border-emerald-700/50 rounded-xl">
        <CheckCircle className="w-6 h-6 text-emerald-400 shrink-0 mt-0.5" />
        <div>
          <p className="text-sm font-medium text-emerald-300">{title}</p>
          <p className="text-xs text-emerald-400/70 mt-0.5">{desc}</p>
        </div>
      </div>
      <Link
        href="/balance"
        className="inline-flex items-center gap-2 text-sm text-gold-400 hover:text-gold-300"
      >
        <ArrowLeft className="w-4 h-4" />
        {backLabel}
      </Link>
    </div>
  );
}

export function DepositFormError({ error }: { readonly error: string }) {
  if (!error) return null;
  return (
    <p className="text-xs text-red-400 bg-red-900/20 border border-red-700/30 rounded-lg px-3 py-2">
      {error}
    </p>
  );
}

export function DepositBackLink({ label }: { readonly label: string }) {
  return (
    <Link
      href="/balance"
      className="p-2 rounded-lg hover:bg-white/5 text-text-muted hover:text-text-primary transition-colors"
      aria-label={label}
    >
      <ArrowLeft className="w-4 h-4" />
    </Link>
  );
}

interface DepositSubmitButtonProps {
  readonly onClick: () => void;
  readonly disabled: boolean;
  readonly busy: boolean;
  readonly busyLabel: string;
  readonly label: string;
  readonly icon: ReactNode;
}

export function DepositSubmitButton({
  onClick,
  disabled,
  busy,
  busyLabel,
  label,
  icon,
}: DepositSubmitButtonProps) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-lg bg-gold-500 hover:bg-gold-400 text-blue-950 text-sm font-semibold transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
    >
      {icon}
      {busy ? busyLabel : label}
    </button>
  );
}

export function providerLabel(provider: string): string {
  return provider === "recurrente" ? "Recurrente" : "PayPal";
}
