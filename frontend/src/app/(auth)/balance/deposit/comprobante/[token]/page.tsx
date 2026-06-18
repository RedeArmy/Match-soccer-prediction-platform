"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { formatAmount } from "@/lib/utils";
import type { PaymentIntentSummary } from "@/lib/api-types";
import { Upload, ArrowLeft, CheckCircle } from "lucide-react";
import { FileDropZone } from "@/components/deposit/FileDropZone";

export default function ComprobanteUploadPage() {
  const params = useParams<{ token: string }>();
  const token = params.token;
  const { getToken } = useAuth();
  const { t } = useI18n();
  const qc = useQueryClient();

  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const { data: intents = [], isLoading } = useQuery<PaymentIntentSummary[]>({
    queryKey: ["payment-intents", "my-all"],
    queryFn: async () => {
      const authToken = await getToken();
      return api.listMyIntents(authToken!);
    },
  });

  // Only allow comprobante upload for pending/expired intents where admin requested it
  const intent = intents.find(
    (i) =>
      i.token === token &&
      (i.status === "pending" || i.status === "expired") &&
      i.comprobante_required,
  );

  async function handleSubmit() {
    if (!file) {
      setError(t("comprobante.noFile"));
      return;
    }
    setUploading(true);
    setError("");
    try {
      const authToken = await getToken();
      const formData = new FormData();
      formData.append("file", file);
      await api.uploadComprobante(authToken!, token, formData);
      await qc.invalidateQueries({ queryKey: ["payment-intents"] });
      setSuccess(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("common.error"));
    } finally {
      setUploading(false);
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="w-6 h-6 border-2 border-gold-400 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!intent) {
    return (
      <div className="space-y-4 text-center py-16">
        <p className="text-text-muted">{t("comprobante.notFound")}</p>
        <Link
          href="/balance"
          className="inline-flex items-center gap-2 text-sm text-gold-400 hover:text-gold-300"
        >
          <ArrowLeft className="w-4 h-4" />
          {t("comprobante.backToBalance")}
        </Link>
      </div>
    );
  }

  if (success || intent?.has_comprobante) {
    return (
      <div className="space-y-6 max-w-md mx-auto">
        <div className="flex items-center gap-3 p-5 bg-emerald-900/40 border border-emerald-700/50 rounded-xl">
          <CheckCircle className="w-6 h-6 text-emerald-400 shrink-0" />
          <div>
            <p className="text-sm font-medium text-emerald-300">
              {t("comprobante.successTitle")}
            </p>
            <p className="text-xs text-emerald-400/70 mt-0.5">
              {t("comprobante.successDesc")}
            </p>
          </div>
        </div>
        <Link
          href="/balance"
          className="inline-flex items-center gap-2 text-sm text-gold-400 hover:text-gold-300"
        >
          <ArrowLeft className="w-4 h-4" />
          {t("comprobante.backToBalance")}
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
          aria-label={t("comprobante.backLabel")}
        >
          <ArrowLeft className="w-4 h-4" />
        </Link>
        <div>
          <h1 className="font-display text-2xl text-white">
            {t("comprobante.title")}
          </h1>
          <p className="text-xs text-text-muted mt-0.5">
            {t("comprobante.subtitlePrefix")} {providerLabel} —{" "}
            <span className="font-score text-gold-400">
              {formatAmount(intent.amount_cents, intent.currency)}
            </span>
          </p>
        </div>
      </div>

      <div className="card space-y-5 p-6">
        <p className="text-sm text-text-secondary">
          {t("comprobante.desc").replace("{provider}", providerLabel)}
        </p>

        <FileDropZone
          file={file}
          preview={preview}
          onFileSelect={(f, p) => {
            setFile(f);
            setPreview(p);
            setError("");
          }}
          onError={setError}
          tooLargeMsg={t("comprobante.fileTooLarge")}
          clickLabel={t("comprobante.clickToSelect")}
          dragLabel={t("comprobante.dragHere")}
          typesLabel={t("comprobante.fileTypes")}
          filePrefix={t("comprobante.filePrefix")}
        />

        {error && (
          <p className="text-xs text-red-400 bg-red-900/20 border border-red-700/30 rounded-lg px-3 py-2">
            {error}
          </p>
        )}

        <button
          onClick={handleSubmit}
          disabled={!file || uploading}
          className="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-lg bg-gold-500 hover:bg-gold-400 text-blue-950 text-sm font-semibold transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Upload className="w-4 h-4" />
          {uploading ? t("comprobante.submitting") : t("comprobante.submit")}
        </button>
      </div>
    </div>
  );
}
