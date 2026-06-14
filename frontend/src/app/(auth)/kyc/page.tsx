"use client";

import { useState, useEffect } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useKYCEffectiveStatus } from "@/hooks/useKYCEffectiveStatus";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";
import { sniffMIME, isAllowedUploadType } from "@/lib/utils";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { LoadingSpinner, LoadingState } from "@/components/shared/LoadingState";
import { FormFeedback } from "@/components/shared/FormFeedback";
import { FormField } from "@/components/shared/FormField";
import { FileUploadField } from "@/components/shared/FileUploadField";
import { SubmitButton } from "@/components/shared/SubmitButton";
import { ShieldCheck, CheckCircle2, Clock, UploadCloud } from "lucide-react";

const statusSteps = ["unverified", "pending", "under_review", "approved"];

type UploadedEntry = { kind: "uploaded"; name: string; id: number };
type PendingEntry = {
  kind: "pending";
  name: string;
  file: File;
  previewUrl?: string;
};
type DocEntry = UploadedEntry | PendingEntry;

export default function KYCPage() {
  const { getToken } = useAuth();
  const { t } = useI18n();
  const qc = useQueryClient();
  const {
    kyc,
    docs: existingDocs,
    isLoading,
    hasPendingReview,
    effectiveStatus,
  } = useKYCEffectiveStatus();
  const [error, setError] = useState("");

  const docTypes = [
    { id: "gov_id", label: t("kyc.docGovId") },
    { id: "selfie", label: t("kyc.docSelfie") },
  ];

  const statusLabels: Record<string, string> = {
    unverified: t("kyc.stepUnverified"),
    pending: t("kyc.stepPending"),
    under_review: t("kyc.stepUnderReview"),
    approved: t("kyc.stepApproved"),
  };

  // prettier-ignore
  const profileFields = [
    { name: "full_name",       label: t("kyc.fieldFullName"),    type: "text", placeholder: t("kyc.phFullName")    },
    { name: "date_of_birth",   label: t("kyc.fieldDob"),         type: "date", placeholder: ""                     },
    { name: "nationality",     label: t("kyc.fieldNationality"), type: "text", placeholder: t("kyc.phNationality") },
    { name: "document_number", label: t("kyc.fieldDocNumber"),   type: "text", placeholder: t("kyc.phDocNumber")   },
    { name: "address_line",    label: t("kyc.fieldAddress"),     type: "text", placeholder: t("kyc.phAddress")     },
    { name: "city",            label: t("kyc.fieldCity"),        type: "text", placeholder: t("kyc.phCity")        },
    { name: "postal_code",     label: t("kyc.fieldPostalCode"),  type: "text", placeholder: t("kyc.phPostalCode")  },
  ];

  // docType → entry (local pending file OR already-uploaded doc)
  const [docEntries, setDocEntries] = useState<Record<string, DocEntry>>({});
  // Per-docType counter incremented on removal to remount the input (allows re-selecting the same file)
  const [resetKeys, setResetKeys] = useState<Record<string, number>>({});
  // True once documents have been successfully submitted
  const [submitted, setSubmitted] = useState(false);

  // Pre-populate uploaded entries from backend
  useEffect(() => {
    if (!existingDocs?.length) return;
    setDocEntries((prev) => {
      const next = { ...prev };
      for (const doc of existingDocs) {
        if (!next[doc.document_type]) {
          next[doc.document_type] = {
            kind: "uploaded",
            name:
              doc.document_type === "gov_id"
                ? t("kyc.docGovIdShort")
                : t("kyc.docSelfieShort"),
            id: doc.id,
          };
        }
      }
      return next;
    });
    // t is stable per locale; existingDocs is the real dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [existingDocs]);

  // Profile form state
  const [form, setForm] = useState({
    full_name: "",
    date_of_birth: "",
    nationality: "",
    document_type: "gov_id",
    document_number: "",
    address_line: "",
    city: "",
    country: "GT",
    postal_code: "",
  });

  const submitProfile = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      return api.submitKYC(token!, form);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["kyc"] });
    },
    onError: (e: Error) => setError(e.message),
  });

  const deleteDoc = useMutation({
    mutationFn: async (docID: number) => {
      const token = await getToken();
      return api.deleteKYCDocument(token!, docID);
    },
    onSuccess: (_, docID) => {
      qc.invalidateQueries({ queryKey: ["kyc-documents"] });
      setDocEntries((prev) => {
        const next = { ...prev };
        for (const key of Object.keys(next)) {
          const e = next[key];
          if (e.kind === "uploaded" && e.id === docID) delete next[key];
        }
        return next;
      });
      setError("");
    },
    onError: (e: Error) => setError(e.message),
  });

  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSendDocuments() {
    setError("");
    const pending = Object.entries(docEntries).filter(
      ([, e]) => e.kind === "pending",
    ) as [string, PendingEntry][];
    if (!pending.length) return;

    setIsSubmitting(true);
    try {
      for (const [docType, entry] of pending) {
        const token = await getToken();
        const fd = new FormData();
        fd.append("document_type", docType);
        fd.append("file", entry.file);
        const doc = await api.uploadKYCDocument(token!, fd);
        if (entry.previewUrl) URL.revokeObjectURL(entry.previewUrl);
        setDocEntries((prev) => ({
          ...prev,
          [docType]: { kind: "uploaded", name: entry.name, id: doc.id },
        }));
      }
      qc.invalidateQueries({ queryKey: ["kyc"] });
      qc.invalidateQueries({ queryKey: ["kyc-documents"] });
      setSubmitted(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("kyc.errUpload"));
    } finally {
      setIsSubmitting(false);
    }
  }

  async function handleFileSelect(
    e: React.ChangeEvent<HTMLInputElement>,
    docType: string,
  ) {
    setError("");
    const file = e.target.files?.[0];
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) {
      setError(t("kyc.errMaxSize"));
      return;
    }
    const mime = await sniffMIME(file);
    if (!isAllowedUploadType(mime)) {
      setError(t("kyc.errFileType"));
      return;
    }
    const previewUrl = mime.startsWith("image/")
      ? URL.createObjectURL(file)
      : undefined;
    setDocEntries((prev) => ({
      ...prev,
      [docType]: { kind: "pending", name: file.name, file, previewUrl },
    }));
  }

  function handleRemove(docType: string) {
    const entry = docEntries[docType];
    if (!entry) return;
    if (entry.kind === "pending") {
      if (entry.previewUrl) URL.revokeObjectURL(entry.previewUrl);
      setDocEntries((prev) => {
        const next = { ...prev };
        delete next[docType];
        return next;
      });
      setResetKeys((prev) => ({
        ...prev,
        [docType]: (prev[docType] ?? 0) + 1,
      }));
    } else {
      deleteDoc.mutate(entry.id);
    }
  }

  if (isLoading) return <LoadingState rows={5} />;

  // hasPendingReview: docs already uploaded but admin hasn't transitioned status yet.
  // showValidating: covers both the in-session upload flow and page reloads.
  const showValidating = submitted || isSubmitting || hasPendingReview;
  const currentStep = statusSteps.indexOf(effectiveStatus);
  const allDocsFilled = docTypes.every(({ id }) => Boolean(docEntries[id]));
  const hasPending = docTypes.some(
    ({ id }) => docEntries[id]?.kind === "pending",
  );
  const isUnderReview = kyc?.status === "under_review";

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <ShieldCheck className="w-6 h-6 text-gold-400" />
        <h1 className="font-display text-3xl text-white">{t("kyc.title")}</h1>
      </div>

      {/* Status stepper */}
      <div className="card p-4">
        <div className="flex items-center justify-between relative">
          {statusSteps.map((step, i) => {
            const done = i <= currentStep;
            const active = i === currentStep;
            const doneActiveClass = active
              ? "bg-green-500 border-green-400 text-white"
              : "bg-green-600 border-green-500 text-white";
            const stepClass = done
              ? doneActiveClass
              : "border-blue-600 text-text-muted";
            return (
              <div
                key={step}
                className="flex flex-col items-center gap-1 flex-1 relative z-10"
              >
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2 ${stepClass}`}
                >
                  {done ? <CheckCircle2 className="w-4 h-4" /> : i + 1}
                </div>
                <span
                  className={`text-[10px] text-center ${done ? "text-green-400" : "text-text-muted"}`}
                >
                  {statusLabels[step]}
                </span>
                {i < statusSteps.length - 1 && (
                  <div
                    className={`absolute top-4 left-1/2 w-full h-0.5 -z-10 ${
                      done && i < currentStep ? "bg-green-500" : "bg-blue-700"
                    }`}
                  />
                )}
              </div>
            );
          })}
        </div>
        {kyc?.status && (
          <div className="text-center mt-3">
            <StatusBadge status={effectiveStatus} />
          </div>
        )}
        {kyc?.rejection_reason && (
          <p className="text-red-400 text-sm text-center mt-2 bg-red-400/10 rounded p-2">
            {t("kyc.rejectionReason")} {kyc.rejection_reason}
          </p>
        )}
      </div>

      {/* Under-review banner */}
      {isUnderReview && (
        <div className="card border border-blue-500/30 bg-blue-500/10 p-5 flex items-start gap-3">
          <Clock className="mt-0.5 h-5 w-5 shrink-0 text-blue-400" />
          <div>
            <p className="text-sm font-medium text-blue-300">
              {t("kyc.reviewBannerTitle")}
            </p>
            <p className="mt-1 text-xs text-text-secondary">
              {t("kyc.reviewBannerBody")}
            </p>
          </div>
        </div>
      )}

      {/* Profile form (only when unverified or rejected) */}
      {(!kyc?.status ||
        kyc.status === "unverified" ||
        kyc.status === "rejected") && (
        <div className="card p-6 space-y-4">
          <h2 className="font-semibold text-text-primary">
            {t("kyc.profileTitle")}
          </h2>

          <div className="grid sm:grid-cols-2 gap-4">
            {profileFields.map(({ name, label, type, placeholder }) => (
              <FormField key={name} label={label}>
                <input
                  type={type}
                  value={form[name as keyof typeof form]}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, [name]: e.target.value }))
                  }
                  placeholder={placeholder}
                  className="input-base"
                />
              </FormField>
            ))}
          </div>

          <SubmitButton
            isPending={submitProfile.isPending}
            onClick={() => {
              setError("");
              submitProfile.mutate();
            }}
          >
            {t("kyc.submitProfile")}
          </SubmitButton>
        </div>
      )}

      {/* Document upload (shown after profile submitted, not yet under review or approved) */}
      {kyc?.status &&
        kyc.status !== "unverified" &&
        kyc.status !== "approved" &&
        !isUnderReview && (
          <div className="card p-6 space-y-4">
            {showValidating ? (
              /* Validating / uploading state — replaces the upload boxes */
              <div className="flex flex-col items-center gap-4 py-8">
                {isSubmitting ? (
                  <LoadingSpinner size={40} />
                ) : (
                  <Clock className="w-10 h-10 text-blue-400" />
                )}
                <div className="text-center">
                  <p className="text-sm font-semibold text-blue-300">
                    {isSubmitting
                      ? t("kyc.uploadingTitle")
                      : t("kyc.validatingTitle")}
                  </p>
                  <p className="mt-1 text-xs text-text-muted">
                    {isSubmitting
                      ? t("kyc.uploadingBody")
                      : t("kyc.validatingBody")}
                  </p>
                </div>
              </div>
            ) : (
              /* Upload boxes */
              <>
                <div>
                  <h2 className="font-semibold text-text-primary">
                    {t("kyc.docsTitle")}
                  </h2>
                  <p className="mt-1 text-xs text-text-muted">
                    {t("kyc.docsHintA")}{" "}
                    <strong className="text-amber-400">
                      {t("kyc.uploadBtn")}
                    </strong>{" "}
                    {t("kyc.docsHintB")}
                  </p>
                </div>

                <div className="grid sm:grid-cols-2 gap-3">
                  {docTypes.map(({ id, label }) => {
                    const entry = docEntries[id];
                    const isPending = entry?.kind === "pending";
                    const previewUrl = isPending ? entry.previewUrl : undefined;
                    return (
                      <FileUploadField
                        key={`${id}-${resetKeys[id] ?? 0}`}
                        label={label}
                        fileName={entry?.name}
                        hasFile={Boolean(entry)}
                        isPending={isPending}
                        pendingLabel={t("kyc.pendingLabel")}
                        previewUrl={previewUrl}
                        onChange={(e) => handleFileSelect(e, id)}
                        onRemove={() => handleRemove(id)}
                      />
                    );
                  })}
                </div>

                {allDocsFilled && hasPending && (
                  <button
                    type="button"
                    onClick={handleSendDocuments}
                    className="btn-gold w-full py-3 text-sm font-semibold flex items-center justify-center gap-2"
                  >
                    <UploadCloud className="w-4 h-4" />
                    {t("kyc.uploadBtn")}
                  </button>
                )}
              </>
            )}
          </div>
        )}

      <FormFeedback error={error} success="" />
    </div>
  );
}
