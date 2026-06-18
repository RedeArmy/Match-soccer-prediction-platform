"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { formatAmount } from "@/lib/utils";
import type { PaymentIntentSummary } from "@/lib/api-types";
import { Upload } from "lucide-react";
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

  if (isLoading) return <DepositLoadingSpinner />;

  if (!intent) {
    return (
      <DepositNotFound
        message={t("comprobante.notFound")}
        backLabel={t("comprobante.backToBalance")}
      />
    );
  }

  if (success || intent?.has_comprobante) {
    return (
      <DepositSuccess
        title={t("comprobante.successTitle")}
        desc={t("comprobante.successDesc")}
        backLabel={t("comprobante.backToBalance")}
      />
    );
  }

  const label = providerLabel(intent.provider);

  return (
    <div className="space-y-6 max-w-md mx-auto">
      <div className="flex items-center gap-3">
        <DepositBackLink label={t("comprobante.backLabel")} />
        <div>
          <h1 className="font-display text-2xl text-white">
            {t("comprobante.title")}
          </h1>
          <p className="text-xs text-text-muted mt-0.5">
            {t("comprobante.subtitlePrefix")} {label} —{" "}
            <span className="font-score text-gold-400">
              {formatAmount(intent.amount_cents, intent.currency)}
            </span>
          </p>
        </div>
      </div>

      <div className="card space-y-5 p-6">
        <p className="text-sm text-text-secondary">
          {t("comprobante.desc").replace("{provider}", label)}
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

        <DepositFormError error={error} />

        <DepositSubmitButton
          onClick={handleSubmit}
          disabled={!file || uploading}
          busy={uploading}
          busyLabel={t("comprobante.submitting")}
          label={t("comprobante.submit")}
          icon={<Upload className="w-4 h-4" />}
        />
      </div>
    </div>
  );
}
