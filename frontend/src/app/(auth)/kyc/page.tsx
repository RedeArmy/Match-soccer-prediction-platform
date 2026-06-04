"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useKYCStatus } from "@/hooks/useKYCStatus";
import { api } from "@/lib/api";
import { sniffMIME, isAllowedUploadType } from "@/lib/utils";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { LoadingState, LoadingSpinner } from "@/components/shared/LoadingState";
import { ShieldCheck, Upload, CheckCircle2 } from "lucide-react";

const docTypes = [
  { id: "gov_id", label: "DPI / Pasaporte (frente)" },
  { id: "selfie", label: "Selfie con DPI" },
  { id: "proof_of_address", label: "Comprobante de domicilio" },
  { id: "proof_of_funds", label: "Comprobante de fondos" },
];

const statusSteps = ["unverified", "submitted", "under_review", "approved"];

const statusLabels: Record<string, string> = {
  unverified: "Sin verificar",
  submitted: "Enviado",
  under_review: "En revisión",
  approved: "Aprobado",
};

export default function KYCPage() {
  const { getToken } = useAuth();
  const qc = useQueryClient();
  const { data: kyc, isLoading } = useKYCStatus();
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

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
      setSuccess("Perfil enviado correctamente.");
    },
    onError: (e: Error) => setError(e.message),
  });

  const uploadDoc = useMutation({
    mutationFn: async ({ file, docType }: { file: File; docType: string }) => {
      const token = await getToken();
      const fd = new FormData();
      fd.append("document_type", docType);
      fd.append("file", file);
      return api.uploadKYCDocument(token!, fd);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["kyc"] });
      setSuccess("Documento cargado correctamente.");
    },
    onError: (e: Error) => setError(e.message),
  });

  async function handleDocUpload(
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
    uploadDoc.mutate({ file, docType });
  }

  if (isLoading) return <LoadingState rows={5} />;

  const currentStep = statusSteps.indexOf(kyc?.status ?? "unverified");

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <ShieldCheck className="w-6 h-6 text-gold-400" />
        <h1 className="font-display text-3xl text-white">VERIFICACIÓN KYC</h1>
      </div>

      {/* Status stepper */}
      <div className="card p-4">
        <div className="flex items-center justify-between relative">
          {statusSteps.map((step, i) => {
            const done = i <= currentStep;
            return (
              <div
                key={step}
                className="flex flex-col items-center gap-1 flex-1 relative z-10"
              >
                <div
                  className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold border-2 ${
                    done
                      ? "bg-gold-500 border-gold-400 text-blue-950"
                      : "border-blue-600 text-text-muted"
                  }`}
                >
                  {done ? <CheckCircle2 className="w-4 h-4" /> : i + 1}
                </div>
                <span
                  className={`text-[10px] text-center ${done ? "text-gold-400" : "text-text-muted"}`}
                >
                  {statusLabels[step]}
                </span>
                {i < statusSteps.length - 1 && (
                  <div
                    className={`absolute top-4 left-1/2 w-full h-0.5 -z-10 ${done && i < currentStep ? "bg-gold-500" : "bg-blue-700"}`}
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

      {/* Submission form (only when unverified or rejected) */}
      {(!kyc?.status ||
        kyc.status === "unverified" ||
        kyc.status === "rejected") && (
        <div className="card p-6 space-y-4">
          <h2 className="font-semibold text-text-primary">
            Información personal
          </h2>

          <div className="grid sm:grid-cols-2 gap-4">
            {[
              {
                name: "full_name",
                label: "Nombre completo",
                type: "text",
                placeholder: "Nombre Apellido",
              },
              {
                name: "date_of_birth",
                label: "Fecha de nacimiento",
                type: "date",
                placeholder: "",
              },
              {
                name: "nationality",
                label: "Nacionalidad",
                type: "text",
                placeholder: "Guatemala",
              },
              {
                name: "document_number",
                label: "Número de documento",
                type: "text",
                placeholder: "0000 00000 0000",
              },
              {
                name: "address_line",
                label: "Dirección",
                type: "text",
                placeholder: "Calle, No.",
              },
              {
                name: "city",
                label: "Ciudad",
                type: "text",
                placeholder: "Ciudad de Guatemala",
              },
              {
                name: "postal_code",
                label: "Código postal",
                type: "text",
                placeholder: "01001",
              },
            ].map(({ name, label, type, placeholder }) => (
              <div key={name}>
                <label className="block text-sm text-text-secondary mb-1">
                  {label}
                </label>
                <input
                  type={type}
                  value={form[name as keyof typeof form]}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, [name]: e.target.value }))
                  }
                  placeholder={placeholder}
                  className="input-base"
                />
              </div>
            ))}
          </div>

          <button
            onClick={() => {
              setError("");
              submitProfile.mutate();
            }}
            disabled={submitProfile.isPending}
            className="btn-gold w-full flex items-center justify-center gap-2"
          >
            {submitProfile.isPending ? (
              <LoadingSpinner size={18} />
            ) : (
              "Enviar información"
            )}
          </button>
        </div>
      )}

      {/* Document upload (only after profile submitted) */}
      {kyc?.status && kyc.status !== "unverified" && (
        <div className="card p-6 space-y-4">
          <h2 className="font-semibold text-text-primary">Documentos</h2>
          <p className="text-xs text-text-muted">
            JPEG, PNG, WebP o PDF — máx. 10 MB por archivo
          </p>

          <div className="grid sm:grid-cols-2 gap-3">
            {docTypes.map(({ id, label }) => (
              <label
                key={id}
                className="flex flex-col items-center gap-2 border-2 border-dashed border-blue-600 rounded-xl p-4 cursor-pointer hover:border-gold-400 transition-colors text-center"
              >
                <Upload className="w-6 h-6 text-blue-400" />
                <span className="text-sm text-text-secondary">{label}</span>
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp,application/pdf"
                  onChange={(e) => handleDocUpload(e, id)}
                  className="sr-only"
                />
              </label>
            ))}
          </div>

          {uploadDoc.isPending && (
            <div className="flex items-center gap-2 text-sm text-text-muted">
              <LoadingSpinner size={16} /> Cargando documento...
            </div>
          )}
        </div>
      )}

      {(error || success) && (
        <div>
          {error && <p className="text-red-400 text-sm">{error}</p>}
          {success && <p className="text-green-400 text-sm">{success}</p>}
        </div>
      )}
    </div>
  );
}
