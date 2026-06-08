"use client";

import { useState, useEffect } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useKYCStatus } from "@/hooks/useKYCStatus";
import { api } from "@/lib/api";
import { sniffMIME, isAllowedUploadType } from "@/lib/utils";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { LoadingSpinner, LoadingState } from "@/components/shared/LoadingState";
import { FormFeedback } from "@/components/shared/FormFeedback";
import { FormField } from "@/components/shared/FormField";
import { FileUploadField } from "@/components/shared/FileUploadField";
import { SubmitButton } from "@/components/shared/SubmitButton";
import { ShieldCheck, CheckCircle2, Clock, UploadCloud } from "lucide-react";

const docTypes = [
  { id: "gov_id", label: "DPI / Pasaporte (frente)" },
  { id: "selfie", label: "Selfie con DPI" },
];

const statusSteps = ["unverified", "pending", "under_review", "approved"];

const statusLabels: Record<string, string> = {
  unverified:   "Sin verificar",
  pending:      "Enviado",
  under_review: "En revisión",
  approved:     "Aprobado",
};

// prettier-ignore
const profileFields: { name: string; label: string; type: string; placeholder: string }[] = [
  { name: "full_name",       label: "Nombre completo",      type: "text", placeholder: "Nombre Apellido"     },
  { name: "date_of_birth",   label: "Fecha de nacimiento",  type: "date", placeholder: ""                    },
  { name: "nationality",     label: "Nacionalidad",         type: "text", placeholder: "Guatemala"           },
  { name: "document_number", label: "Número de documento",  type: "text", placeholder: "0000 00000 0000"     },
  { name: "address_line",    label: "Dirección",            type: "text", placeholder: "Calle, No."          },
  { name: "city",            label: "Ciudad",               type: "text", placeholder: "Ciudad de Guatemala"  },
  { name: "postal_code",     label: "Código postal",        type: "text", placeholder: "01001"               },
];

type UploadedEntry = { kind: "uploaded"; name: string; id: number };
type PendingEntry  = { kind: "pending";  name: string; file: File; previewUrl?: string };
type DocEntry = UploadedEntry | PendingEntry;

export default function KYCPage() {
  const { getToken } = useAuth();
  const qc = useQueryClient();
  const { data: kyc, isLoading } = useKYCStatus();
  const [error, setError] = useState("");

  // docType → entry (local pending file OR already-uploaded doc)
  const [docEntries, setDocEntries] = useState<Record<string, DocEntry>>({});
  // Per-docType counter incremented on removal to remount the input (allows re-selecting the same file)
  const [resetKeys, setResetKeys] = useState<Record<string, number>>({});
  // True once documents have been successfully submitted
  const [submitted, setSubmitted] = useState(false);

  // Load already-submitted documents from backend
  const { data: existingDocs } = useQuery({
    queryKey: ["kyc-documents"],
    queryFn: async () => {
      const token = await getToken();
      return api.getKYCDocuments(token!);
    },
    enabled: Boolean(kyc?.status && kyc.status !== "unverified"),
  });

  // Pre-populate uploaded entries from backend
  useEffect(() => {
    if (!existingDocs?.length) return;
    setDocEntries((prev) => {
      const next = { ...prev };
      for (const doc of existingDocs) {
        if (!next[doc.document_type]) {
          next[doc.document_type] = { kind: "uploaded", name: doc.document_type === "gov_id" ? "DPI / Pasaporte" : "Selfie con DPI", id: doc.id };
        }
      }
      return next;
    });
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
      setError(e instanceof Error ? e.message : "Error al enviar documentos.");
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
      setError("Máx. 10 MB por documento");
      return;
    }
    const mime = await sniffMIME(file);
    if (!isAllowedUploadType(mime)) {
      setError("Tipo no permitido. Usa JPEG, PNG, WebP o PDF.");
      return;
    }
    const previewUrl = mime.startsWith("image/") ? URL.createObjectURL(file) : undefined;
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
      // Increment reset key so the file input remounts and the same file can be re-selected
      setResetKeys((prev) => ({ ...prev, [docType]: (prev[docType] ?? 0) + 1 }));
    } else {
      deleteDoc.mutate(entry.id);
    }
  }

  if (isLoading) return <LoadingState rows={5} />;

  const currentStep = statusSteps.indexOf(kyc?.status ?? "unverified");
  const allDocsFilled = docTypes.every(({ id }) => Boolean(docEntries[id]));
  const hasPending    = docTypes.some(({ id }) => docEntries[id]?.kind === "pending");
  const isUnderReview = kyc?.status === "under_review";
  const showValidating = submitted || isSubmitting;

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <ShieldCheck className="w-6 h-6 text-gold-400" />
        <h1 className="font-display text-3xl text-white">VERIFICACIÓN DE IDENTIDAD</h1>
      </div>

      {/* Status stepper */}
      <div className="card p-4">
        <div className="flex items-center justify-between relative">
          {statusSteps.map((step, i) => {
            const done   = i <= currentStep;
            const active = i === currentStep;
            return (
              <div
                key={step}
                className="flex flex-col items-center gap-1 flex-1 relative z-10"
              >
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2 ${
                    done
                      ? active
                        ? "bg-green-500 border-green-400 text-white"
                        : "bg-green-600 border-green-500 text-white"
                      : "border-blue-600 text-text-muted"
                  }`}
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
            <StatusBadge status={kyc.status} />
          </div>
        )}
        {kyc?.rejection_reason && (
          <p className="text-red-400 text-sm text-center mt-2 bg-red-400/10 rounded p-2">
            Motivo de rechazo: {kyc.rejection_reason}
          </p>
        )}
      </div>

      {/* Under-review banner */}
      {isUnderReview && (
        <div className="card border border-blue-500/30 bg-blue-500/10 p-5 flex items-start gap-3">
          <Clock className="mt-0.5 h-5 w-5 shrink-0 text-blue-400" />
          <div>
            <p className="text-sm font-medium text-blue-300">Documentos en revisión</p>
            <p className="mt-1 text-xs text-text-secondary">
              Hemos recibido tus documentos y están siendo revisados por nuestro equipo.
              Te notificaremos cuando se complete la verificación.
            </p>
          </div>
        </div>
      )}

      {/* Profile form (only when unverified or rejected) */}
      {(!kyc?.status || kyc.status === "unverified" || kyc.status === "rejected") && (
        <div className="card p-6 space-y-4">
          <h2 className="font-semibold text-text-primary">Información personal</h2>

          <div className="grid sm:grid-cols-2 gap-4">
            {profileFields.map(({ name, label, type, placeholder }) => (
              <FormField key={name} label={label}>
                <input
                  type={type}
                  value={form[name as keyof typeof form]}
                  onChange={(e) => setForm((f) => ({ ...f, [name]: e.target.value }))}
                  placeholder={placeholder}
                  className="input-base"
                />
              </FormField>
            ))}
          </div>

          <SubmitButton
            isPending={submitProfile.isPending}
            onClick={() => { setError(""); submitProfile.mutate(); }}
          >
            Enviar información
          </SubmitButton>
        </div>
      )}

      {/* Document upload (shown after profile submitted, not yet under review) */}
      {kyc?.status && kyc.status !== "unverified" && !isUnderReview && (
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
                  {isSubmitting ? "Enviando documentos..." : "Validación en progreso"}
                </p>
                <p className="mt-1 text-xs text-text-muted">
                  {isSubmitting
                    ? "Por favor espera mientras se suben tus archivos."
                    : "Hemos recibido tus documentos. Te notificaremos cuando se complete la verificación."}
                </p>
              </div>
            </div>
          ) : (
            /* Upload boxes */
            <>
              <div>
                <h2 className="font-semibold text-text-primary">Documentos</h2>
                <p className="mt-1 text-xs text-text-muted">
                  Selecciona los archivos — se precargan localmente sin enviarse.
                  Cuando ambos estén listos, presiona{" "}
                  <strong className="text-amber-400">Cargar archivos</strong> para guardarlos.
                  JPEG, PNG, WebP o PDF — máx. 10 MB por archivo.
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
                  Cargar archivos
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
