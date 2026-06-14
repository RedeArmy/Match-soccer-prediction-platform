"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { LoadingState, LoadingSpinner } from "@/components/shared/LoadingState";
import { CheckCircle, XCircle } from "lucide-react";

const statuses = [
  "",
  "pending",
  "under_review",
  "escalated",
  "approved",
  "rejected",
  "expired",
];
const tierOptions = [1, 2, 3];

export default function AdminKYCQueuePage() {
  const { getToken } = useAuth();
  const qc = useQueryClient();
  const [filterStatus, setFilterStatus] = useState("");
  const [rejectTarget, setRejectTarget] = useState<number | null>(null);
  const [rejectReason, setRejectReason] = useState("");
  const [approveTarget, setApproveTarget] = useState<number | null>(null);
  const [approveTier, setApproveTier] = useState(2);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-kyc-queue", filterStatus],
    queryFn: async () => {
      const token = await getToken();
      return api.adminGetKYCQueue(token!, filterStatus || undefined);
    },
  });

  const approve = useMutation({
    mutationFn: async ({
      profileID,
      tier,
    }: {
      profileID: number;
      tier: number;
    }) => {
      const token = await getToken();
      return api.adminApproveKYC(token!, profileID, tier);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-kyc-queue"] });
      setApproveTarget(null);
      setApproveTier(2);
    },
  });

  const reject = useMutation({
    mutationFn: async ({
      profileID,
      reason,
    }: {
      profileID: number;
      reason: string;
    }) => {
      const token = await getToken();
      return api.adminRejectKYC(token!, profileID, reason);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin-kyc-queue"] });
      setRejectTarget(null);
      setRejectReason("");
    },
  });

  const profiles = data ?? [];

  return (
    <div className="space-y-5">
      <h1 className="font-display text-3xl text-white">KYC QUEUE</h1>

      {/* Filter */}
      <div className="flex gap-1 p-1 bg-blue-900 rounded-xl w-fit">
        {statuses.map((s) => (
          <button
            key={s}
            onClick={() => setFilterStatus(s)}
            className={`px-3 py-1.5 rounded-lg text-xs transition-colors ${
              filterStatus === s
                ? "bg-blue-700 text-white font-medium"
                : "text-text-muted hover:text-text-secondary"
            }`}
          >
            {s || "Todos"}
          </button>
        ))}
      </div>

      {isLoading ? (
        <LoadingState rows={6} />
      ) : (
        <div className="card divide-y divide-blue-800/50">
          {!profiles.length && (
            <p className="text-text-muted text-sm text-center py-8">
              No hay perfiles en esta cola
            </p>
          )}
          {profiles.map((p) => (
            <div
              key={p.id}
              className="flex items-center justify-between gap-3 px-4 py-3"
            >
              <div className="min-w-0 flex-1">
                <p className="text-sm text-text-primary font-medium">
                  {p.full_name ?? `Perfil #${p.id}`}
                </p>
                {p.user_email && (
                  <p className="text-xs text-text-muted truncate">
                    {p.user_email}
                  </p>
                )}
                <div className="flex gap-3 text-xs text-text-muted mt-0.5">
                  {p.submitted_at && <span>{formatDate(p.submitted_at)}</span>}
                  <span>Tier {p.tier}</span>
                  {p.risk_score !== null && <span>Riesgo: {p.risk_score}</span>}
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <StatusBadge status={p.status} size="sm" />
                {(p.status === "pending" || p.status === "under_review") && (
                  <>
                    <button
                      onClick={() => {
                        setApproveTarget(p.id);
                        setApproveTier(2);
                      }}
                      disabled={approve.isPending}
                      className="p-1.5 rounded hover:bg-green-400/10 text-green-400 transition-colors"
                      title="Aprobar"
                    >
                      <CheckCircle className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => {
                        setRejectTarget(p.id);
                        setRejectReason("");
                      }}
                      className="p-1.5 rounded hover:bg-red-400/10 text-red-400 transition-colors"
                      title="Rechazar"
                    >
                      <XCircle className="w-4 h-4" />
                    </button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Approve dialog */}
      {approveTarget !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-blue-950/80 backdrop-blur-sm px-4">
          <div className="card p-6 w-full max-w-sm space-y-4">
            <h3 className="font-semibold text-white">Aprobar perfil KYC</h3>
            <div className="space-y-2">
              <p className="text-xs text-text-muted">
                Asignar tier de verificación
              </p>
              <div className="flex gap-2">
                {tierOptions.map((t) => (
                  <button
                    key={t}
                    onClick={() => setApproveTier(t)}
                    className={`flex-1 py-2 rounded-lg text-sm font-medium transition-colors ${
                      approveTier === t
                        ? "bg-green-500 text-white"
                        : "bg-blue-900 text-text-muted hover:text-white"
                    }`}
                  >
                    Tier {t}
                  </button>
                ))}
              </div>
              <p className="text-xs text-text-muted">
                {approveTier === 1 &&
                  "Básico — acceso limitado a funciones de pago"}
                {approveTier === 2 &&
                  "Estándar — límites de transacción normales"}
                {approveTier === 3 &&
                  "Premium — sin restricciones de transacción"}
              </p>
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => setApproveTarget(null)}
                className="btn-ghost flex-1 text-sm py-2"
              >
                Cancelar
              </button>
              <button
                onClick={() =>
                  approve.mutate({
                    profileID: approveTarget,
                    tier: approveTier,
                  })
                }
                disabled={approve.isPending}
                className="flex-1 text-sm py-2 bg-green-500 hover:bg-green-400 text-white rounded-lg transition-colors flex items-center justify-center gap-1"
              >
                {approve.isPending ? <LoadingSpinner size={16} /> : "Aprobar"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Reject dialog */}
      {rejectTarget !== null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-blue-950/80 backdrop-blur-sm px-4">
          <div className="card p-6 w-full max-w-sm space-y-4">
            <h3 className="font-semibold text-white">Rechazar perfil KYC</h3>
            <textarea
              value={rejectReason}
              onChange={(e) => setRejectReason(e.target.value)}
              placeholder="Motivo de rechazo..."
              rows={3}
              className="input-base resize-none"
            />
            <div className="flex gap-2">
              <button
                onClick={() => setRejectTarget(null)}
                className="btn-ghost flex-1 text-sm py-2"
              >
                Cancelar
              </button>
              <button
                onClick={() =>
                  reject.mutate({
                    profileID: rejectTarget,
                    reason: rejectReason,
                  })
                }
                disabled={reject.isPending || !rejectReason.trim()}
                className="flex-1 text-sm py-2 bg-red-500 hover:bg-red-400 text-white rounded-lg transition-colors flex items-center justify-center gap-1"
              >
                {reject.isPending ? <LoadingSpinner size={16} /> : "Rechazar"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
