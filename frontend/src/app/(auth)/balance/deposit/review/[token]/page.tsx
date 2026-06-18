"use client";

import { useState, useRef } from "react";
import { useParams } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { PaymentIntentSummary } from "@/lib/api-types";
import { ArrowLeft, CheckCircle, ImageIcon, Send } from "lucide-react";

const MAX_FILE_BYTES = 10 * 1024 * 1024; // 10 MB

function formatAmount(cents: number, currency: string): string {
  if (currency === "USD") return `$${(cents / 100).toFixed(2)} USD`;
  return `Q${(cents / 100).toFixed(2)}`;
}

export default function ReviewRequestPage() {
  const params = useParams<{ token: string }>();
  const intentToken = params.token;
  const { getToken } = useAuth();
  const { t } = useI18n();
  const qc = useQueryClient();

  const [notes, setNotes] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const { data: intents = [], isLoading } = useQuery<PaymentIntentSummary[]>({
    queryKey: ["payment-intents", "my-all"],
    queryFn: async () => {
      const authToken = await getToken();
      return api.listMyIntents(authToken!);
    },
  });

  const intent = intents.find((i) => i.token === intentToken);

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (!f) return;
    if (f.size > MAX_FILE_BYTES) {
      setError(t("review.fileTooLarge"));
      return;
    }
    setFile(f);
    setError("");
    if (f.type.startsWith("image/")) {
      const reader = new FileReader();
      reader.onload = () => setPreview(reader.result as string);
      reader.readAsDataURL(f);
    } else {
      setPreview(null);
    }
  }

  async function handleSubmit() {
    if (!notes.trim()) {
      setError(t("review.notesLabel"));
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      const authToken = await getToken();
      const formData = new FormData();
      formData.append("notes", notes.trim());
      if (file) formData.append("file", file);
      await api.resubmitForReview(authToken!, intentToken, formData);
      await qc.invalidateQueries({ queryKey: ["payment-intents"] });
      setSuccess(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setSubmitting(false);
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="w-6 h-6 border-2 border-gold-400 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!intent || intent.status !== "rejected") {
    return (
      <div className="space-y-4 text-center py-16">
        <p className="text-text-muted">{t("review.notFound")}</p>
        <Link
          href="/balance"
          className="inline-flex items-center gap-2 text-sm text-gold-400 hover:text-gold-300"
        >
          <ArrowLeft className="w-4 h-4" />
          {t("review.backToBalance")}
        </Link>
      </div>
    );
  }

  if (success) {
    return (
      <div className="space-y-6 max-w-md mx-auto">
        <div className="flex items-start gap-3 p-5 bg-emerald-900/40 border border-emerald-700/50 rounded-xl">
          <CheckCircle className="w-6 h-6 text-emerald-400 shrink-0 mt-0.5" />
          <div>
            <p className="text-sm font-medium text-emerald-300">
              {t("review.successTitle")}
            </p>
            <p className="text-xs text-emerald-400/70 mt-0.5">
              {t("review.successDesc")}
            </p>
          </div>
        </div>
        <Link
          href="/balance"
          className="inline-flex items-center gap-2 text-sm text-gold-400 hover:text-gold-300"
        >
          <ArrowLeft className="w-4 h-4" />
          {t("review.backToBalance")}
        </Link>
      </div>
    );
  }

  const providerLabel =
    intent.provider === "recurrente" ? "Recurrente" : "PayPal";

  return (
    <div className="space-y-6 max-w-md mx-auto">
      <div className="flex items-center gap-3">
        <Link
          href="/balance"
          className="p-2 rounded-lg hover:bg-white/5 text-text-muted hover:text-text-primary transition-colors"
          aria-label={t("review.backLabel")}
        >
          <ArrowLeft className="w-4 h-4" />
        </Link>
        <div>
          <h1 className="font-display text-2xl text-white">
            {t("review.title")}
          </h1>
          <p className="text-xs text-text-muted mt-0.5">
            {t("comprobante.subtitlePrefix")} {providerLabel}{" "}
            {t("review.subtitleStatus")} —{" "}
            <span className="font-score text-red-400">
              {formatAmount(intent.amount_cents, intent.currency)}
            </span>
          </p>
        </div>
      </div>

      <div className="card space-y-5 p-6">
        <p className="text-sm text-text-secondary">{t("review.desc")}</p>

        {/* Description */}
        <div className="space-y-1.5">
          <label
            htmlFor="review-notes"
            className="block text-sm font-medium text-white/70"
          >
            {t("review.notesLabel")} <span className="text-red-400">*</span>
          </label>
          <textarea
            id="review-notes"
            rows={4}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder={t("review.notesPh")}
            className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-gold-400/50"
          />
          {notes.length < 10 && notes.length > 0 && (
            <p className="text-xs text-white/40">
              {t("review.notesMinChars")} ({notes.length}/10)
            </p>
          )}
        </div>

        {/* Optional file */}
        <div className="space-y-2">
          <label className="block text-sm font-medium text-white/70">
            {t("review.fileLabel")}{" "}
            <span className="text-white/30 font-normal text-xs">
              {t("review.fileOptional")}
            </span>
          </label>

          <div
            className="border-2 border-dashed border-white/20 rounded-xl p-5 text-center cursor-pointer hover:border-gold-400/50 hover:bg-white/[0.02] transition-colors"
            onClick={() => fileInputRef.current?.click()}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              e.preventDefault();
              const f = e.dataTransfer.files?.[0];
              if (f) {
                const synth = {
                  target: { files: [f] },
                } as unknown as React.ChangeEvent<HTMLInputElement>;
                handleFileChange(synth);
              }
            }}
          >
            {preview ? (
              <img
                src={preview}
                alt="Vista previa"
                className="max-h-40 mx-auto rounded-lg object-contain"
              />
            ) : (
              <div className="flex flex-col items-center gap-2 text-text-muted">
                <ImageIcon className="w-8 h-8 opacity-40" />
                <p className="text-sm text-text-secondary">
                  {t("review.clickToSelect")}
                </p>
                <p className="text-xs opacity-60">{t("review.fileTypes")}</p>
              </div>
            )}
          </div>

          <input
            ref={fileInputRef}
            type="file"
            accept="image/*,application/pdf"
            className="hidden"
            onChange={handleFileChange}
          />

          {file && (
            <p className="text-xs text-text-muted">
              {t("review.filePrefix")}{" "}
              <span className="text-text-secondary">{file.name}</span> (
              {(file.size / 1024).toFixed(0)} KB)
            </p>
          )}
        </div>

        {error && (
          <p className="text-xs text-red-400 bg-red-900/20 border border-red-700/30 rounded-lg px-3 py-2">
            {error}
          </p>
        )}

        <button
          onClick={handleSubmit}
          disabled={!notes.trim() || notes.trim().length < 10 || submitting}
          className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-lg bg-gold-500 hover:bg-gold-400 text-blue-950 text-sm font-semibold transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Send className="w-4 h-4" />
          {submitting ? t("review.submitting") : t("review.submit")}
        </button>
      </div>
    </div>
  );
}
