"use client";

import {
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  X,
  XCircle,
  AlertTriangle,
  ExternalLink,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { LoadingState } from "@/components/shared/LoadingState";

// ── AdminPageHeader ───────────────────────────────────────────────────────────

interface AdminPageHeaderProps {
  readonly title: string;
  readonly subtitle: string;
  readonly onRefresh?: () => void;
  readonly isLoading?: boolean;
}

export function AdminPageHeader({
  title,
  subtitle,
  onRefresh,
  isLoading = false,
}: AdminPageHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <h1 className="text-2xl font-bold text-white">{title}</h1>
        <p className="text-sm text-white/50 mt-1">{subtitle}</p>
      </div>
      {onRefresh && (
        <button
          onClick={onRefresh}
          disabled={isLoading}
          className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white text-sm transition-colors disabled:opacity-50"
        >
          <RefreshCw className={cn("h-4 w-4", isLoading && "animate-spin")} />
          Actualizar
        </button>
      )}
    </div>
  );
}

// ── AdminModalOverlay ─────────────────────────────────────────────────────────

interface AdminModalOverlayProps {
  readonly onClose: () => void;
  readonly children: React.ReactNode;
  readonly scrollable?: boolean;
}

export function AdminModalOverlay({
  onClose,
  children,
  scrollable,
}: AdminModalOverlayProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        className={cn(
          "relative z-10 w-full max-w-md bg-[#1a1a2e] border border-white/10 rounded-xl shadow-2xl p-6 space-y-5",
          scrollable && "max-h-[90vh] overflow-y-auto",
        )}
      >
        {children}
      </div>
    </div>
  );
}

// ── ModalHeader ───────────────────────────────────────────────────────────────

export function ModalHeader({
  title,
  onClose,
}: Readonly<{ title: string; onClose: () => void }>) {
  return (
    <div className="flex items-start justify-between gap-4">
      <h2 className="text-lg font-semibold text-white">{title}</h2>
      <button
        onClick={onClose}
        className="text-white/40 hover:text-white/80 transition-colors shrink-0 mt-0.5"
      >
        <X className="h-5 w-5" />
      </button>
    </div>
  );
}

// ── ModalErrorLine ────────────────────────────────────────────────────────────

export function ModalErrorLine({ error }: Readonly<{ error: string }>) {
  if (!error) return null;
  return (
    <p className="text-red-400 text-sm flex items-center gap-1.5">
      <AlertTriangle className="h-4 w-4 shrink-0" />
      {error}
    </p>
  );
}

// ── ModalCancelButton ─────────────────────────────────────────────────────────

export function ModalCancelButton({
  onClose,
  disabled,
}: Readonly<{ onClose: () => void; disabled: boolean }>) {
  return (
    <button
      onClick={onClose}
      disabled={disabled}
      className="px-4 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 text-sm font-medium transition-colors disabled:opacity-50"
    >
      Cancelar
    </button>
  );
}

// ── InfoRow ───────────────────────────────────────────────────────────────────

export function InfoRow({
  label,
  value,
}: Readonly<{ label: string; value: React.ReactNode }>) {
  return (
    <div className="flex justify-between items-start gap-4 py-1.5 border-b border-white/5 last:border-0">
      <span className="text-white/50 text-sm shrink-0">{label}</span>
      <span className="text-white text-sm font-medium text-right">{value}</span>
    </div>
  );
}

// ── AdminPagination ───────────────────────────────────────────────────────────

function getPageNumbers(current: number, total: number): (number | "...")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
  const pages = new Set<number>([1, total, current]);
  if (current > 1) pages.add(current - 1);
  if (current < total) pages.add(current + 1);
  const sorted = Array.from(pages).sort((a, b) => a - b);
  const result: (number | "...")[] = [];
  for (let i = 0; i < sorted.length; i++) {
    if (i > 0 && sorted[i] - sorted[i - 1] > 1) result.push("...");
    result.push(sorted[i]);
  }
  return result;
}

interface AdminPaginationProps {
  readonly page: number;
  readonly pageCount: number;
  readonly rangeStart: number;
  readonly rangeEnd: number;
  readonly total: number;
  readonly itemLabel: string;
  readonly onPageChange: (page: number) => void;
}

export function AdminPagination({
  page,
  pageCount,
  rangeStart,
  rangeEnd,
  total,
  itemLabel,
  onPageChange,
}: AdminPaginationProps) {
  if (pageCount <= 1) return null;
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-white/40">
        {rangeStart}–{rangeEnd} de {total} {itemLabel}
      </span>
      <div className="flex items-center gap-1">
        <button
          onClick={() => onPageChange(Math.max(1, page - 1))}
          disabled={page === 1}
          className="p-1.5 rounded-md hover:bg-white/10 text-white/60 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        {getPageNumbers(page, pageCount).map((n, i, arr) =>
          n === "..." ? (
            <span key={`gap-${arr[i + 1]}`} className="px-2 text-white/30">
              …
            </span>
          ) : (
            <button
              key={n}
              onClick={() => onPageChange(n)}
              aria-current={n === page ? "page" : undefined}
              className={cn(
                "w-8 h-8 rounded-md text-sm font-medium transition-colors",
                n === page
                  ? "bg-emerald-500/20 text-emerald-400 ring-1 ring-emerald-500/40"
                  : "hover:bg-white/10 text-white/60",
              )}
            >
              {n}
            </button>
          ),
        )}
        <button
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
          disabled={page === pageCount}
          className="p-1.5 rounded-md hover:bg-white/10 text-white/60 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}

// ── AdminTabBar ───────────────────────────────────────────────────────────────

interface AdminTabBarProps<TKey extends string> {
  readonly tabs: ReadonlyArray<{ readonly key: TKey; readonly label: string }>;
  readonly activeTab: TKey;
  readonly counts: Readonly<Record<string, number>>;
  readonly onTabChange: (key: TKey) => void;
  readonly activeButtonClass: string;
  readonly activeBadgeClass: string;
}

export function AdminTabBar<TKey extends string>({
  tabs,
  activeTab,
  counts,
  onTabChange,
  activeButtonClass,
  activeBadgeClass,
}: AdminTabBarProps<TKey>) {
  return (
    <div className="flex flex-wrap gap-2">
      {tabs.map((t) => {
        const count = counts[t.key] ?? 0;
        const isActive = activeTab === t.key;
        return (
          <button
            key={t.key}
            onClick={() => onTabChange(t.key)}
            className={cn(
              "flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors",
              isActive
                ? activeButtonClass
                : "bg-white/5 text-white/60 hover:bg-white/10 hover:text-white",
            )}
          >
            {t.label}
            <span
              className={cn(
                "text-xs px-1.5 py-0.5 rounded-full tabular-nums",
                isActive ? activeBadgeClass : "bg-white/10 text-white/40",
              )}
            >
              {count}
            </span>
          </button>
        );
      })}
    </div>
  );
}

// ── AdminContentState ─────────────────────────────────────────────────────────

interface AdminContentStateProps {
  readonly isLoading: boolean;
  readonly error: unknown;
  readonly isEmpty: boolean;
  readonly emptyTitle: string;
  readonly emptyMessage: string;
  readonly errorMessage: string;
  readonly children: React.ReactNode;
}

export function AdminContentState({
  isLoading,
  error,
  isEmpty,
  emptyTitle,
  emptyMessage,
  errorMessage,
  children,
}: AdminContentStateProps) {
  if (isLoading) return <LoadingState />;
  if (error)
    return (
      <div className="text-center py-12 text-red-400 text-sm">
        {errorMessage}
      </div>
    );
  if (isEmpty)
    return (
      <div className="text-center py-16 text-white/40">
        <p className="text-lg font-medium">{emptyTitle}</p>
        <p className="text-sm mt-1">{emptyMessage}</p>
      </div>
    );
  return <>{children}</>;
}

// ── AdminRefreshButton ────────────────────────────────────────────────────────

export function AdminRefreshButton({
  onClick,
  disabled,
}: Readonly<{ onClick: () => void; disabled: boolean }>) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white text-sm transition-colors disabled:opacity-50"
    >
      Actualizar
    </button>
  );
}

// ── RejectNotesTextarea ───────────────────────────────────────────────────────

interface RejectNotesTextareaProps {
  readonly id: string;
  readonly value: string;
  readonly onChange: (v: string) => void;
  readonly placeholder: string;
}

export function RejectNotesTextarea({
  id,
  value,
  onChange,
  placeholder,
}: RejectNotesTextareaProps) {
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-sm font-medium text-white/70">
        Motivo de rechazo <span className="text-red-400">*</span>
      </label>
      <textarea
        id={id}
        rows={3}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder:text-white/30 resize-none focus:outline-none focus:ring-1 focus:ring-red-500/50"
      />
    </div>
  );
}

// ── RejectWarningBox ──────────────────────────────────────────────────────────

export function RejectWarningBox() {
  return (
    <div className="flex items-start gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/20">
      <XCircle className="h-4 w-4 text-red-400 mt-0.5 shrink-0" />
      <p className="text-red-300 text-xs">
        El pago será marcado como rechazado. No se acreditará ningún monto al
        usuario.
      </p>
    </div>
  );
}

// ── ProofViewLink ─────────────────────────────────────────────────────────────

export function ProofViewLink({
  href,
  title,
}: {
  readonly href: string;
  readonly title?: string;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-center gap-1 text-blue-400 hover:text-blue-300 text-xs transition-colors"
      title={title}
    >
      Ver <ExternalLink className="h-3 w-3" />
    </a>
  );
}

// ── ComprobanteViewLink ───────────────────────────────────────────────────────

export function ComprobanteViewLink({ href }: { readonly href: string | null }) {
  if (!href) return null;
  return (
    <InfoRow label="Comprobante" value={<ProofViewLink href={href} />} />
  );
}
