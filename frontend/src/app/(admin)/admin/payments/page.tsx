"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle,
  XCircle,
  ExternalLink,
  Bell,
  MessageSquare,
} from "lucide-react";
import { api } from "@/lib/api";
import type { PaymentIntentAdminResponse, PaymentIntentStatus } from "@/lib/api-types";
import { formatDateTime, formatRelative } from "@/lib/utils";
import { BankTransfersTab } from "@/components/admin/BankTransfersTab";
import {
  AdminPageHeader,
  AdminModalOverlay,
  AdminPagination,
  AdminContentState,
  ModalHeader,
  ModalCancelButton,
  ModalErrorLine,
  InfoRow,
} from "@/components/admin/shared";

const PAGE_SIZE = 15;

type MainTab = "recurrente" | "paypal" | "transferencias";

const MAIN_TABS: { key: MainTab; label: string }[] = [
  { key: "recurrente", label: "Recurrente" },
  { key: "paypal", label: "PayPal" },
  { key: "transferencias", label: "Transferencias" },
];

// ── Status dot ────────────────────────────────────────────────────────────────

function StatusDot({ status }: { readonly status: PaymentIntentStatus }) {
  if (status === "captured") {
    return (
      <span
        className="inline-block w-2.5 h-2.5 rounded-full bg-emerald-400"
        title="Acreditado"
      />
    );
  }
  if (status === "rejected") {
    return (
      <span
        className="inline-block w-2.5 h-2.5 rounded-full bg-red-400"
        title="Rechazado"
      />
    );
  }
  return (
    <span
      className="inline-block w-2.5 h-2.5 rounded-full bg-amber-400"
      title={status === "expired" ? "Vencido" : "Pendiente"}
    />
  );
}

function formatAmount(cents: number, currency: string): string {
  if (currency === "USD") {
    return `$${(cents / 100).toFixed(2)} USD`;
  }
  return `Q${(cents / 100).toFixed(2)}`;
}

// ── Credit modal ──────────────────────────────────────────────────────────────

interface CreditModalProps {
  readonly item: PaymentIntentAdminResponse;
  readonly notes: string;
  readonly onNotesChange: (v: string) => void;
  readonly isBusy: boolean;
  readonly error: string;
  readonly onSubmit: () => void;
  readonly onClose: () => void;
  readonly comprobanteUrl: string | null;
}

function CreditModal({
  item,
  notes,
  onNotesChange,
  isBusy,
  error,
  onSubmit,
  onClose,
  comprobanteUrl,
}: CreditModalProps) {
  return (
    <>
      <ModalHeader title={`Acreditar Pago #${item.id}`} onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-1">
        <InfoRow
          label="Usuario"
          value={<span className="font-mono text-xs">#{item.user_id}</span>}
        />
        <InfoRow
          label="Monto"
          value={
            <span className="font-semibold">
              {formatAmount(item.amount_cents, item.currency)}
            </span>
          }
        />
        <InfoRow label="Proveedor" value={item.provider} />
        <InfoRow label="Estado" value={item.status} />
        {comprobanteUrl && (
          <InfoRow
            label="Comprobante"
            value={
              <a
                href={comprobanteUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-blue-400 hover:text-blue-300 transition-colors text-xs"
              >
                Ver <ExternalLink className="h-3 w-3" />
              </a>
            }
          />
        )}
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-amber-500/10 border border-amber-500/20">
        <CheckCircle className="h-4 w-4 text-amber-400 mt-0.5 shrink-0" />
        <p className="text-amber-300 text-xs">
          Se acreditará{" "}
          <strong>{formatAmount(item.amount_cents, item.currency)}</strong> al
          balance del usuario.{" "}
          {item.currency === "USD" && (
            <span>El monto en USD se convertirá al tipo de cambio actual.</span>
          )}{" "}
          Esta acción es irreversible.
        </p>
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="credit-notes"
          className="block text-sm font-medium text-white/70"
        >
          Notas (opcional)
        </label>
        <textarea
          id="credit-notes"
          rows={2}
          value={notes}
          onChange={(e) => onNotesChange(e.target.value)}
          placeholder="Observaciones para el registro..."
          className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-emerald-500/50"
        />
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onSubmit}
          disabled={isBusy}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <CheckCircle className="h-4 w-4" />
          {isBusy ? "Procesando..." : "Acreditar"}
        </button>
      </div>
    </>
  );
}

// ── Reject modal ──────────────────────────────────────────────────────────────

interface RejectIntentModalProps {
  readonly item: PaymentIntentAdminResponse;
  readonly notes: string;
  readonly onNotesChange: (v: string) => void;
  readonly isBusy: boolean;
  readonly error: string;
  readonly onSubmit: () => void;
  readonly onClose: () => void;
}

function RejectIntentModal({
  item,
  notes,
  onNotesChange,
  isBusy,
  error,
  onSubmit,
  onClose,
}: RejectIntentModalProps) {
  return (
    <>
      <ModalHeader title={`Rechazar Pago #${item.id}`} onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-1">
        <InfoRow
          label="Usuario"
          value={<span className="font-mono text-xs">#{item.user_id}</span>}
        />
        <InfoRow
          label="Monto"
          value={formatAmount(item.amount_cents, item.currency)}
        />
        <InfoRow label="Proveedor" value={item.provider} />
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="reject-intent-notes"
          className="block text-sm font-medium text-white/70"
        >
          Motivo de rechazo <span className="text-red-400">*</span>
        </label>
        <textarea
          id="reject-intent-notes"
          rows={3}
          value={notes}
          onChange={(e) => onNotesChange(e.target.value)}
          placeholder="Describe el motivo del rechazo..."
          className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-red-500/50"
        />
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/20">
        <XCircle className="h-4 w-4 text-red-400 mt-0.5 shrink-0" />
        <p className="text-red-300 text-xs">
          El pago será marcado como rechazado. No se acreditará ningún monto al
          usuario.
        </p>
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onSubmit}
          disabled={isBusy || !notes.trim()}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-red-600 hover:bg-red-500 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <XCircle className="h-4 w-4" />
          {isBusy ? "Rechazando..." : "Rechazar"}
        </button>
      </div>
    </>
  );
}

// ── PaymentIntentsTab ─────────────────────────────────────────────────────────

type IntentModalState =
  | { kind: "credit"; item: PaymentIntentAdminResponse }
  | { kind: "reject"; item: PaymentIntentAdminResponse }
  | null;

function PaymentIntentsTab({ provider }: { readonly provider: "recurrente" | "paypal" }) {
  const { getToken } = useAuth();
  const qc = useQueryClient();
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<IntentModalState>(null);
  const [modalError, setModalError] = useState("");
  const [notes, setNotes] = useState("");

  const queryKey = ["admin", "payment-intents", provider, page];

  const { data, isLoading, error, refetch } = useQuery({
    queryKey,
    queryFn: async () => {
      const token = await getToken();
      return api.adminListPaymentIntents(token!, {
        provider,
        page,
        limit: PAGE_SIZE,
      });
    },
  });

  const intents = data?.data ?? [];
  const total = data?.total ?? 0;
  const pageCount = Math.ceil(total / PAGE_SIZE);
  const rangeStart = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const rangeEnd = Math.min(page * PAGE_SIZE, total);

  function openCredit(item: PaymentIntentAdminResponse) {
    setModal({ kind: "credit", item });
    setModalError("");
    setNotes("");
  }

  function openReject(item: PaymentIntentAdminResponse) {
    setModal({ kind: "reject", item });
    setModalError("");
    setNotes("");
  }

  function closeModal() {
    setModal(null);
    setModalError("");
    setNotes("");
  }

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: ["admin", "payment-intents", provider] });

  const requestComprobanteMutation = useMutation({
    mutationFn: async ({ id }: { id: number }) => {
      const token = await getToken();
      return api.adminRequestComprobante(token!, id);
    },
    onSuccess: () => invalidate(),
    onError: () => {},
  });

  const creditMutation = useMutation({
    mutationFn: async ({ id, notes }: { id: number; notes?: string }) => {
      const token = await getToken();
      return api.adminCreditPaymentIntent(token!, id, notes);
    },
    onSuccess: () => {
      invalidate();
      closeModal();
    },
    onError: (e: unknown) => {
      setModalError(e instanceof Error ? e.message : "Error al acreditar el pago");
    },
  });

  const rejectMutation = useMutation({
    mutationFn: async ({ id, notes }: { id: number; notes: string }) => {
      const token = await getToken();
      return api.adminRejectPaymentIntent(token!, id, notes);
    },
    onSuccess: () => {
      invalidate();
      closeModal();
    },
    onError: (e: unknown) => {
      setModalError(e instanceof Error ? e.message : "Error al rechazar el pago");
    },
  });

  const isBusy =
    creditMutation.isPending ||
    rejectMutation.isPending ||
    requestComprobanteMutation.isPending;

  const canAct = (status: PaymentIntentStatus) =>
    status === "pending" || status === "expired" || status === "under_review";

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <p className="text-sm text-white/50">{total} pagos en total</p>
        <button
          onClick={() => refetch()}
          disabled={isLoading}
          className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white text-sm transition-colors disabled:opacity-50"
        >
          Actualizar
        </button>
      </div>

      <AdminContentState
        isLoading={isLoading}
        error={error}
        isEmpty={intents.length === 0}
        emptyTitle="Sin pagos"
        emptyMessage={`No hay pagos de ${provider} registrados.`}
        errorMessage={`Error al cargar pagos de ${provider}.`}
      >
        <>
          <div className="overflow-x-auto rounded-xl border border-white/10">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/10 bg-white/5 text-white/50 text-left">
                  <th className="px-4 py-3 font-medium">#</th>
                  <th className="px-4 py-3 font-medium">Usuario</th>
                  <th className="px-4 py-3 font-medium">Monto</th>
                  <th className="px-4 py-3 font-medium">Estado</th>
                  <th className="px-4 py-3 font-medium">Fecha</th>
                  <th className="px-4 py-3 font-medium">Comprobante</th>
                  <th className="px-4 py-3 font-medium text-right">Acciones</th>
                </tr>
              </thead>
              <tbody>
                {intents.map((item) => (
                  <tr
                    key={item.id}
                    className="border-b border-white/5 hover:bg-white/[0.03] transition-colors"
                  >
                    <td className="px-4 py-3 text-white/40 font-mono text-xs">
                      #{item.id}
                    </td>

                    <td className="px-4 py-3">
                      <span className="text-white/70 font-mono text-xs bg-white/5 px-2 py-0.5 rounded">
                        #{item.user_id}
                      </span>
                    </td>

                    <td className="px-4 py-3 tabular-nums">
                      <div className="font-semibold text-white">
                        {formatAmount(item.amount_cents, item.currency)}
                      </div>
                      {item.status === "captured" && item.currency === "USD" && (
                        <div className="text-white/30 text-xs">convertido a GTQ</div>
                      )}
                    </td>

                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <StatusDot status={item.status} />
                        <div>
                          <span className="text-xs text-white/60 capitalize">
                            {item.status === "captured"
                              ? "Acreditado"
                              : item.status === "rejected"
                                ? "Rechazado"
                                : item.status === "expired"
                                  ? "Vencido"
                                  : item.status === "under_review"
                                    ? "En revisión"
                                    : "Pendiente"}
                          </span>
                          {item.status === "under_review" && item.user_notes && (
                            <p
                              className="text-xs text-white/40 italic truncate max-w-[140px]"
                              title={item.user_notes}
                            >
                              <MessageSquare className="inline h-3 w-3 mr-0.5" />
                              {item.user_notes}
                            </p>
                          )}
                        </div>
                      </div>
                    </td>

                    <td
                      className="px-4 py-3 text-white/50 text-xs"
                      title={formatDateTime(item.created_at)}
                    >
                      {formatRelative(item.created_at)}
                    </td>

                    <td className="px-4 py-3">
                      {item.has_comprobante ? (
                        <a
                          href={api.adminPaymentIntentComprobanteUrl(item.id)}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1 text-blue-400 hover:text-blue-300 text-xs transition-colors"
                        >
                          Ver <ExternalLink className="h-3 w-3" />
                        </a>
                      ) : (
                        <span className="text-white/25 text-xs">—</span>
                      )}
                    </td>

                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2 flex-wrap">
                        {item.status === "pending" && !item.comprobante_required && (
                          <button
                            onClick={() =>
                              requestComprobanteMutation.mutate({ id: item.id })
                            }
                            disabled={requestComprobanteMutation.isPending}
                            title="Solicitar comprobante al usuario"
                            className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-amber-500/15 hover:bg-amber-500/25 text-amber-400 text-xs font-medium transition-colors disabled:opacity-50"
                          >
                            <Bell className="h-3.5 w-3.5" />
                            Comprobante
                          </button>
                        )}
                        {canAct(item.status) && (
                          <>
                            <button
                              onClick={() => openCredit(item)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-emerald-500/15 hover:bg-emerald-500/25 text-emerald-400 text-xs font-medium transition-colors"
                            >
                              <CheckCircle className="h-3.5 w-3.5" />
                              Acreditar
                            </button>
                            <button
                              onClick={() => openReject(item)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-red-500/15 hover:bg-red-500/25 text-red-400 text-xs font-medium transition-colors"
                            >
                              <XCircle className="h-3.5 w-3.5" />
                              Rechazar
                            </button>
                          </>
                        )}
                        {!canAct(item.status) && item.review_notes && (
                          <span
                            className="text-white/30 text-xs italic max-w-[120px] truncate"
                            title={item.review_notes}
                          >
                            {item.review_notes}
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <AdminPagination
            page={page}
            pageCount={pageCount}
            rangeStart={rangeStart}
            rangeEnd={rangeEnd}
            total={total}
            itemLabel="pagos"
            onPageChange={setPage}
          />
        </>
      </AdminContentState>

      {modal && (
        <AdminModalOverlay onClose={closeModal} scrollable>
          {modal.kind === "credit" ? (
            <CreditModal
              item={modal.item}
              notes={notes}
              onNotesChange={setNotes}
              isBusy={isBusy}
              error={modalError}
              onSubmit={() =>
                creditMutation.mutate({
                  id: modal.item.id,
                  notes: notes.trim() || undefined,
                })
              }
              onClose={closeModal}
              comprobanteUrl={
                modal.item.has_comprobante
                  ? api.adminPaymentIntentComprobanteUrl(modal.item.id)
                  : null
              }
            />
          ) : (
            <RejectIntentModal
              item={modal.item}
              notes={notes}
              onNotesChange={setNotes}
              isBusy={isBusy}
              error={modalError}
              onSubmit={() => {
                if (!notes.trim()) {
                  setModalError("El motivo de rechazo es requerido");
                  return;
                }
                rejectMutation.mutate({ id: modal.item.id, notes: notes.trim() });
              }}
              onClose={closeModal}
            />
          )}
        </AdminModalOverlay>
      )}
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function AdminPaymentsPage() {
  const [tab, setTab] = useState<MainTab>("recurrente");

  return (
    <div className="space-y-6">
      <AdminPageHeader
        title="Pagos"
        subtitle="Pagos de Recurrente, PayPal y transferencias bancarias"
        onRefresh={() => {}}
        isLoading={false}
      />

      {/* Main tab bar */}
      <div className="flex gap-1 p-1 bg-white/5 rounded-lg w-fit">
        {MAIN_TABS.map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setTab(key)}
            className={
              tab === key
                ? "px-4 py-2 rounded-md text-sm font-medium bg-blue-600 text-white transition-colors"
                : "px-4 py-2 rounded-md text-sm font-medium text-white/60 hover:text-white hover:bg-white/10 transition-colors"
            }
          >
            {label}
          </button>
        ))}
      </div>

      {tab === "recurrente" && <PaymentIntentsTab provider="recurrente" />}
      {tab === "paypal" && <PaymentIntentsTab provider="paypal" />}
      {tab === "transferencias" && <BankTransfersTab />}
    </div>
  );
}
