"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { formatAmount } from "@/lib/utils";
import type { PaymentIntentSummary } from "@/lib/api-types";
import { Send } from "lucide-react";
import { FileDropZone } from "@/components/deposit/FileDropZone";
import {
  DepositLoadingSpinner,
  DepositNotFound,
  DepositSuccess,
  DepositFormError,
  DepositBackLink,
  DepositSubmitButton,
  providerLabel,
} from "@/components/deposit/DepositPageState";

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

  const { data: intents = [], isLoading } = useQuery<PaymentIntentSummary[]>({
    queryKey: ["payment-intents", "my-all"],
    queryFn: async () => {
      const authToken = await getToken();
      return api.listMyIntents(authToken!);
    },
  });

  const intent = intents.find((i) => i.token === intentToken);

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

  if (isLoading) return <DepositLoadingSpinner />;

  if (intent?.status !== "rejected") {
    return (
      <DepositNotFound
        message={t("review.notFound")}
        backLabel={t("review.backToBalance")}
      />
    );
  }

  if (success) {
    return (
      <DepositSuccess
        title={t("review.successTitle")}
        desc={t("review.successDesc")}
        backLabel={t("review.backToBalance")}
      />
    );
  }

  const label = providerLabel(intent.provider);

  return (
    <div className="space-y-6 max-w-md mx-auto">
      <div className="flex items-center gap-3">
        <DepositBackLink label={t("review.backLabel")} />
        <div>
          <h1 className="font-display text-2xl text-white">
            {t("review.title")}
          </h1>
          <p className="text-xs text-text-muted mt-0.5">
            {t("comprobante.subtitlePrefix")} {label}{" "}
            {t("review.subtitleStatus")} —{" "}
            <span className="font-score text-red-400">
              {formatAmount(intent.amount_cents, intent.currency)}
            </span>
          </p>
        </div>
      </div>

      <div className="card space-y-5 p-6">
        <p className="text-sm text-text-secondary">{t("review.desc")}</p>

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

        <div className="space-y-2">
          <label className="block text-sm font-medium text-white/70">
            {t("review.fileLabel")}{" "}
            <span className="text-white/30 font-normal text-xs">
              {t("review.fileOptional")}
            </span>
          </label>

          <FileDropZone
            file={file}
            preview={preview}
            onFileSelect={(f, p) => {
              setFile(f);
              setPreview(p);
              setError("");
            }}
            onError={setError}
            tooLargeMsg={t("review.fileTooLarge")}
            clickLabel={t("review.clickToSelect")}
            typesLabel={t("review.fileTypes")}
            filePrefix={t("review.filePrefix")}
          />
        </div>

        <DepositFormError error={error} />

        <DepositSubmitButton
          onClick={handleSubmit}
          disabled={!notes.trim() || notes.trim().length < 10 || submitting}
          busy={submitting}
          busyLabel={t("review.submitting")}
          label={t("review.submit")}
          icon={<Send className="w-4 h-4" />}
        />
      </div>
    </div>
  );
}
