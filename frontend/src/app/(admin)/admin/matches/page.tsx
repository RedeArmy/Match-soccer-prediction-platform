"use client";

import { type ReactNode, useState, useMemo } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Play,
  Edit3,
  CheckCircle,
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  Ban,
  RefreshCw,
} from "lucide-react";
import { api } from "@/lib/api";
import type {
  MatchResponse,
  MatchStatus,
  TournamentSlotResponse,
} from "@/lib/api-types";
import { cn, formatDateTime, formatRelative } from "@/lib/utils";
import { StatusBadge } from "@/components/shared/StatusBadge";
import {
  AdminModalOverlay,
  AdminContentState,
  AdminTabBar,
  ModalHeader,
  ModalCancelButton,
  ModalErrorLine,
} from "@/components/admin/shared";

type TabKey = "all" | "today" | "bracket" | MatchStatus;

const PAGE_SIZE = 15;

const TABS: { key: TabKey; label: string }[] = [
  { key: "all", label: "Todos" },
  { key: "today", label: "Hoy" },
  { key: "scheduled", label: "Programados" },
  { key: "in_progress", label: "En Curso" },
  { key: "finished", label: "Finalizados" },
  { key: "cancelled", label: "Cancelados" },
  { key: "bracket", label: "Llave" },
];

const PHASE_LABEL: Record<string, string> = {
  group_stage: "Fase de Grupos",
  round_of_32: "Ronda de 32",
  round_of_16: "Ronda de 16",
  quarter_final: "Cuartos de Final",
  semi_final: "Semifinal",
  third_place: "Tercer Lugar",
  final: "Final",
};

const WIN_METHOD_OPTIONS = [
  { value: "", label: "— No aplica / Normal —" },
  { value: "extra_time", label: "Tiempo extra" },
  { value: "penalties", label: "Penales" },
];

function phaseLabel(phase: string | null): string {
  if (!phase) return "—";
  return PHASE_LABEL[phase] ?? phase;
}

function scoreLabel(match: MatchResponse): string {
  if (match.home_score === null || match.away_score === null) return "— : —";
  return `${match.home_score} : ${match.away_score}`;
}

// ── Start confirm modal ───────────────────────────────────────────────────────

interface StartModalProps {
  readonly match: MatchResponse;
  readonly isBusy: boolean;
  readonly error: string;
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}

function StartModal({
  match,
  isBusy,
  error,
  onConfirm,
  onClose,
}: StartModalProps) {
  return (
    <>
      <ModalHeader title="Iniciar Partido" onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-3 space-y-1.5">
        <p className="text-white font-semibold text-center text-lg">
          {match.home_team} <span className="text-white/40 mx-2">vs</span>{" "}
          {match.away_team}
        </p>
        <p className="text-white/50 text-xs text-center">
          {phaseLabel(match.phase)}
        </p>
        {match.kickoff_at && (
          <p className="text-white/50 text-xs text-center">
            {formatDateTime(match.kickoff_at)}
          </p>
        )}
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-amber-500/10 border border-amber-500/20">
        <AlertTriangle className="h-4 w-4 text-amber-400 mt-0.5 shrink-0" />
        <p className="text-amber-300 text-xs">
          Esto cambiará el estado del partido a <strong>en curso</strong>. Esta
          acción es manual y sobreescribe el ciclo automático.
        </p>
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onConfirm}
          disabled={isBusy}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-green-600 hover:bg-green-500 text-white text-sm font-medium transition-colors disabled:opacity-50"
        >
          <Play className="h-4 w-4" />
          {isBusy ? "Iniciando..." : "Iniciar Partido"}
        </button>
      </div>
    </>
  );
}

// ── Cancel confirm modal ──────────────────────────────────────────────────────

interface CancelModalProps {
  readonly match: MatchResponse;
  readonly isBusy: boolean;
  readonly error: string;
  readonly onConfirm: () => void;
  readonly onClose: () => void;
}

function CancelModal({
  match,
  isBusy,
  error,
  onConfirm,
  onClose,
}: CancelModalProps) {
  return (
    <>
      <ModalHeader title="Cancelar Partido" onClose={onClose} />

      <div className="bg-white/5 rounded-lg px-4 py-3 space-y-1.5">
        <p className="text-white font-semibold text-center text-lg">
          {match.home_team} <span className="text-white/40 mx-2">vs</span>{" "}
          {match.away_team}
        </p>
        <p className="text-white/50 text-xs text-center">{match.phase}</p>
      </div>

      <div className="flex items-start gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/20">
        <AlertTriangle className="h-4 w-4 text-red-400 mt-0.5 shrink-0" />
        <p className="text-red-300 text-xs">
          El partido quedará como <strong>cancelado</strong>. Esta acción no
          puede revertirse desde la UI — contacta a un desarrollador si
          necesitas restaurarlo.
        </p>
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onConfirm}
          disabled={isBusy}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-red-600 hover:bg-red-500 text-white text-sm font-medium transition-colors disabled:opacity-50"
        >
          <Ban className="h-4 w-4" />
          {isBusy ? "Cancelando..." : "Cancelar Partido"}
        </button>
      </div>
    </>
  );
}

// ── Result update modal ───────────────────────────────────────────────────────

interface ResultForm {
  homeScore: string;
  awayScore: string;
  winMethod: string;
}

interface ResultModalProps {
  readonly match: MatchResponse;
  readonly form: ResultForm;
  readonly onFormChange: (f: ResultForm) => void;
  readonly isBusy: boolean;
  readonly error: string;
  readonly onSubmit: () => void;
  readonly onClose: () => void;
  readonly mode: "result" | "correct";
}

function ResultModal({
  match,
  form,
  onFormChange,
  isBusy,
  error,
  onSubmit,
  onClose,
  mode,
}: ResultModalProps) {
  const isKnockout = match.phase && match.phase !== "group_stage";
  const homeInt = Number.parseInt(form.homeScore, 10);
  const awayInt = Number.parseInt(form.awayScore, 10);
  const scoresValid =
    !Number.isNaN(homeInt) &&
    !Number.isNaN(awayInt) &&
    homeInt >= 0 &&
    awayInt >= 0;
  const modalTitle =
    mode === "correct" ? "Corregir Resultado" : "Actualizar Resultado";

  return (
    <>
      <ModalHeader title={modalTitle} onClose={onClose} />

      <p className="text-white/60 text-sm text-center">
        {match.home_team} <span className="text-white/30 mx-1">vs</span>{" "}
        {match.away_team}
        <span className="ml-2 text-white/30 text-xs">
          ({phaseLabel(match.phase)})
        </span>
      </p>

      <div className="grid grid-cols-3 items-center gap-4">
        <div className="space-y-1.5">
          <label
            htmlFor="result-home"
            className="block text-xs font-medium text-white/50 text-center"
          >
            {match.home_team}
          </label>
          <input
            id="result-home"
            type="number"
            min={0}
            max={99}
            value={form.homeScore}
            onChange={(e) =>
              onFormChange({ ...form, homeScore: e.target.value })
            }
            className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2.5 text-2xl font-bold text-white text-center focus:outline-none focus:ring-1 focus:ring-blue-500/50"
          />
        </div>
        <div className="text-center text-white/30 text-2xl font-bold">:</div>
        <div className="space-y-1.5">
          <label
            htmlFor="result-away"
            className="block text-xs font-medium text-white/50 text-center"
          >
            {match.away_team}
          </label>
          <input
            id="result-away"
            type="number"
            min={0}
            max={99}
            value={form.awayScore}
            onChange={(e) =>
              onFormChange({ ...form, awayScore: e.target.value })
            }
            className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2.5 text-2xl font-bold text-white text-center focus:outline-none focus:ring-1 focus:ring-blue-500/50"
          />
        </div>
      </div>

      {isKnockout && (
        <div className="space-y-1.5">
          <label
            htmlFor="win-method"
            className="block text-sm font-medium text-white/70"
          >
            Método de victoria
          </label>
          <select
            id="win-method"
            value={form.winMethod}
            onChange={(e) =>
              onFormChange({ ...form, winMethod: e.target.value })
            }
            className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-blue-500/50"
          >
            {WIN_METHOD_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
      )}

      <div className="flex items-start gap-2 p-3 rounded-lg bg-blue-500/10 border border-blue-500/20">
        <AlertTriangle className="h-4 w-4 text-blue-400 mt-0.5 shrink-0" />
        <p className="text-blue-300 text-xs">
          {mode === "correct" ? (
            <>
              El marcador será <strong>corregido</strong> y las predicciones
              serán <strong>re-puntuadas</strong> automáticamente.
            </>
          ) : (
            <>
              El marcador se actualizará y el partido quedará como{" "}
              <strong>finalizado</strong>. Las predicciones serán puntuadas en
              el próximo ciclo del worker.
            </>
          )}
        </p>
      </div>

      <ModalErrorLine error={error} />

      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={onSubmit}
          disabled={isBusy || !scoresValid}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <CheckCircle className="h-4 w-4" />
          {isBusy ? "Guardando..." : modalTitle}
        </button>
      </div>
    </>
  );
}

// ── Bracket tab ──────────────────────────────────────────────────────────────

interface ConfirmSlotModalProps {
  readonly slot: TournamentSlotResponse;
  readonly isBusy: boolean;
  readonly error: string;
  readonly onConfirm: (team: string) => void;
  readonly onClose: () => void;
}

function ConfirmSlotModal({
  slot,
  isBusy,
  error,
  onConfirm,
  onClose,
}: ConfirmSlotModalProps) {
  const [team, setTeam] = useState(slot.team ?? "");
  return (
    <>
      <ModalHeader title="Corregir equipo en bracket" onClose={onClose} />
      <div className="space-y-0.5">
        <p className="font-mono text-[10px] text-white/40">{slot.label}</p>
        <p className="text-sm text-white/60">{slot.description}</p>
        <p className="text-[11px] text-amber-400/70">
          Esta corrección sobreescribe el valor asignado automáticamente.
        </p>
      </div>
      <div className="space-y-1.5">
        <label
          htmlFor="slot-team"
          className="block text-xs font-medium text-white/50"
        >
          Nombre del equipo
        </label>
        <input
          id="slot-team"
          type="text"
          value={team}
          onChange={(e) => setTeam(e.target.value)}
          placeholder="Ej. Argentina"
          className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-blue-500/50"
        />
      </div>
      <ModalErrorLine error={error} />
      <div className="flex justify-end gap-3">
        <ModalCancelButton onClose={onClose} disabled={isBusy} />
        <button
          onClick={() => {
            if (team.trim()) onConfirm(team.trim());
          }}
          disabled={isBusy || !team.trim()}
          className="flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <CheckCircle className="h-4 w-4" />
          {isBusy ? "Guardando..." : "Corregir"}
        </button>
      </div>
    </>
  );
}

interface BracketTabProps {
  readonly getToken: () => Promise<string | null>;
}

function BracketTab({ getToken }: BracketTabProps) {
  const qc = useQueryClient();
  const [confirmSlot, setConfirmSlot] = useState<TournamentSlotResponse | null>(
    null,
  );
  const [slotError, setSlotError] = useState("");

  const {
    data: slots = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["admin", "slots"],
    queryFn: async () => {
      const token = await getToken();
      return api.getSlots(token);
    },
    refetchInterval: 30_000,
  });

  const confirmMutation = useMutation({
    mutationFn: async ({ slotId, team }: { slotId: number; team: string }) => {
      const token = await getToken();
      return api.adminConfirmSlot(token!, slotId, team);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin", "slots"] });
      void qc.invalidateQueries({ queryKey: ["tournament-slots"] });
      setConfirmSlot(null);
      setSlotError("");
    },
    onError: (e: unknown) => {
      setSlotError(e instanceof Error ? e.message : "Error al confirmar slot");
    },
  });

  if (isLoading) {
    return (
      <div className="py-8 text-center text-white/40 text-sm">
        Cargando slots…
      </div>
    );
  }
  if (error) {
    return (
      <div className="py-8 text-center text-red-400 text-sm">
        Error al cargar los slots.
      </div>
    );
  }
  if (!slots.length) {
    return (
      <div className="py-8 text-center text-white/40 text-sm">
        No hay slots de llave registrados.
      </div>
    );
  }

  return (
    <>
      <div className="overflow-x-auto rounded-xl border border-white/10">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/10 bg-white/5 text-white/50 text-left">
              <th className="px-4 py-3 font-medium">Slot</th>
              <th className="px-4 py-3 font-medium">Descripción</th>
              <th className="px-4 py-3 font-medium">Equipo</th>
              <th className="px-4 py-3 font-medium">Confirmado</th>
              <th className="px-4 py-3 font-medium text-right">Acción</th>
            </tr>
          </thead>
          <tbody>
            {slots.map((slot) => (
              <tr
                key={slot.id}
                className="border-b border-white/5 hover:bg-white/[0.03] transition-colors"
              >
                <td className="px-4 py-3 font-mono text-xs text-white/60">
                  {slot.label}
                </td>
                <td className="px-4 py-3 text-white/60 text-xs">
                  {slot.description || "—"}
                </td>
                <td className="px-4 py-3 text-white font-medium">
                  {slot.team ?? (
                    <span className="text-white/30 italic">Sin equipo</span>
                  )}
                </td>
                <td className="px-4 py-3 text-xs">
                  {slot.confirmed_at ? (
                    <span className="text-green-400">
                      ✓ {new Date(slot.confirmed_at).toLocaleDateString("sv")}
                    </span>
                  ) : (
                    <span className="text-white/30">—</span>
                  )}
                </td>
                <td className="px-4 py-3 text-right">
                  <button
                    onClick={() => {
                      setConfirmSlot(slot);
                      setSlotError("");
                    }}
                    className="flex items-center gap-1 ml-auto px-2.5 py-1 rounded-md bg-blue-500/15 hover:bg-blue-500/25 text-blue-400 text-xs font-medium transition-colors"
                  >
                    <Edit3 className="h-3.5 w-3.5" />
                    {slot.team ? "Corregir" : "Confirmar"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {confirmSlot && (
        <AdminModalOverlay onClose={() => setConfirmSlot(null)}>
          <ConfirmSlotModal
            slot={confirmSlot}
            isBusy={confirmMutation.isPending}
            error={slotError}
            onConfirm={(team) =>
              confirmMutation.mutate({ slotId: confirmSlot.id, team })
            }
            onClose={() => {
              setConfirmSlot(null);
              setSlotError("");
            }}
          />
        </AdminModalOverlay>
      )}
    </>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────────

type ModalState =
  | { kind: "start"; match: MatchResponse }
  | { kind: "result"; match: MatchResponse }
  | { kind: "correct"; match: MatchResponse }
  | { kind: "cancel"; match: MatchResponse }
  | null;

export default function AdminMatchesPage() {
  const { getToken } = useAuth();
  const qc = useQueryClient();

  const [tab, setTab] = useState<TabKey>("all");
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<ModalState>(null);
  const [modalError, setModalError] = useState("");
  const [syncCooldown, setSyncCooldown] = useState(false);
  const [syncResult, setSyncResult] = useState<{
    linked: number;
    kickoffs_updated: number;
    scores_corrected: number;
  } | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [resultForm, setResultForm] = useState<ResultForm>({
    homeScore: "",
    awayScore: "",
    winMethod: "",
  });
  const [syncStartDate, setSyncStartDate] = useState("");
  const [syncEndDate, setSyncEndDate] = useState("");

  const {
    data: matches = [],
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: ["admin", "matches"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMatches(token!);
    },
  });

  const { data: clockData } = useQuery({
    queryKey: ["system", "clock"],
    queryFn: async () => {
      const res = await fetch("/api/v1/system/clock");
      if (!res.ok) return null;
      const data = (await res.json()) as { now: string };
      return data.now;
    },
    staleTime: 30_000,
  });
  const todayStr = useMemo(() => {
    const base = clockData ? new Date(clockData) : new Date();
    return base.toLocaleDateString("sv");
  }, [clockData]);

  const filtered = useMemo(() => {
    if (tab === "all") return matches;
    if (tab === "today")
      return matches.filter((m) => m.kickoff_at?.slice(0, 10) === todayStr);
    return matches.filter((m) => m.status === tab);
  }, [matches, tab, todayStr]);

  const counts = useMemo(() => {
    const result: Record<string, number> = { all: matches.length };
    for (const m of matches) result[m.status] = (result[m.status] ?? 0) + 1;
    result.today = matches.filter(
      (m) => m.kickoff_at?.slice(0, 10) === todayStr,
    ).length;
    return result;
  }, [matches, todayStr]);

  const paginated = useMemo(() => {
    if (tab !== "all") return filtered;
    const start = (page - 1) * PAGE_SIZE;
    return filtered.slice(start, start + PAGE_SIZE);
  }, [filtered, tab, page]);

  const totalPages =
    tab === "all" ? Math.max(1, Math.ceil(filtered.length / PAGE_SIZE)) : 1;

  function changeTab(next: TabKey) {
    setTab(next);
    setPage(1);
  }

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: ["admin", "matches"] });

  // After a result is confirmed, the worker auto-confirms bracket slots
  // asynchronously. Delay-invalidate slot queries to pick up the update once
  // the worker has had time to process the MatchFinished event (~4 s).
  const invalidateSlots = () => {
    setTimeout(() => {
      void qc.invalidateQueries({ queryKey: ["admin", "slots"] });
      void qc.invalidateQueries({ queryKey: ["tournament-slots"] });
    }, 4_000);
  };

  function openStart(match: MatchResponse) {
    setModal({ kind: "start", match });
    setModalError("");
  }

  function openResult(match: MatchResponse) {
    setModal({ kind: "result", match });
    setModalError("");
    setResultForm({
      homeScore: match.home_score === null ? "" : String(match.home_score),
      awayScore: match.away_score === null ? "" : String(match.away_score),
      winMethod: match.win_method ?? "",
    });
  }

  function openCorrect(match: MatchResponse) {
    setModal({ kind: "correct", match });
    setModalError("");
    setResultForm({
      homeScore: match.home_score === null ? "" : String(match.home_score),
      awayScore: match.away_score === null ? "" : String(match.away_score),
      winMethod: match.win_method ?? "",
    });
  }

  function openCancel(match: MatchResponse) {
    setModal({ kind: "cancel", match });
    setModalError("");
  }

  function closeModal() {
    setModal(null);
    setModalError("");
  }

  const startMutation = useMutation({
    mutationFn: async (id: number) => {
      const token = await getToken();
      return api.adminStartMatch(token!, id);
    },
    onSuccess: () => {
      invalidate();
      closeModal();
    },
    onError: (e: unknown) => {
      setModalError(
        e instanceof Error ? e.message : "Error al iniciar el partido",
      );
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number;
      data: { home_score: number; away_score: number; win_method?: string };
    }) => {
      const token = await getToken();
      return api.adminUpdateMatchResult(token!, id, data);
    },
    onSuccess: () => {
      invalidate();
      invalidateSlots();
      closeModal();
    },
    onError: (e: unknown) => {
      setModalError(
        e instanceof Error ? e.message : "Error al actualizar el resultado",
      );
    },
  });

  const correctMutation = useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number;
      data: { home_score: number; away_score: number; win_method?: string };
    }) => {
      const token = await getToken();
      return api.adminCorrectMatchResult(token!, id, data);
    },
    onSuccess: () => {
      invalidate();
      invalidateSlots();
      closeModal();
    },
    onError: (e: unknown) => {
      setModalError(
        e instanceof Error ? e.message : "Error al corregir el resultado",
      );
    },
  });

  const cancelMutation = useMutation({
    mutationFn: async (id: number) => {
      const token = await getToken();
      return api.adminCancelMatch(token!, id);
    },
    onSuccess: () => {
      invalidate();
      closeModal();
    },
    onError: (e: unknown) => {
      setModalError(
        e instanceof Error ? e.message : "Error al cancelar el partido",
      );
    },
  });

  const dailySyncMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      return api.adminTriggerDailySync(
        token!,
        syncStartDate || undefined,
        syncEndDate || undefined,
      );
    },
  });

  async function handleSync() {
    setSyncCooldown(true);
    setSyncResult(null);
    setSyncError(null);
    try {
      const result = await dailySyncMutation.mutateAsync();
      await refetch();
      setSyncResult({
        linked: result.linked,
        kickoffs_updated: result.kickoffs_updated,
        scores_corrected: result.scores_corrected,
      });
    } catch (e) {
      setSyncError(e instanceof Error ? e.message : "Error al sincronizar");
    } finally {
      setTimeout(() => setSyncCooldown(false), 5_000);
    }
  }

  const isBusy =
    startMutation.isPending ||
    updateMutation.isPending ||
    correctMutation.isPending ||
    cancelMutation.isPending;

  let emptyMsg = `No hay partidos con estado "${tab}".`;
  if (tab === "all") emptyMsg = "No hay partidos registrados.";
  if (tab === "today") emptyMsg = "No hay partidos programados para hoy.";

  function submitResult() {
    if (modal?.kind !== "result" && modal?.kind !== "correct") return;
    const home = Number.parseInt(resultForm.homeScore, 10);
    const away = Number.parseInt(resultForm.awayScore, 10);
    if (Number.isNaN(home) || Number.isNaN(away) || home < 0 || away < 0) {
      setModalError("Marcador inválido");
      return;
    }
    const data: {
      home_score: number;
      away_score: number;
      win_method?: string;
    } = {
      home_score: home,
      away_score: away,
    };
    if (resultForm.winMethod) data.win_method = resultForm.winMethod;
    if (modal.kind === "correct") {
      correctMutation.mutate({ id: modal.match.id, data });
    } else {
      updateMutation.mutate({ id: modal.match.id, data });
    }
  }

  const resultMode: "result" | "correct" =
    modal?.kind === "correct" ? "correct" : "result";

  let modalContent: ReactNode = null;
  if (modal?.kind === "start") {
    modalContent = (
      <StartModal
        match={modal.match}
        isBusy={isBusy}
        error={modalError}
        onConfirm={() => startMutation.mutate(modal.match.id)}
        onClose={closeModal}
      />
    );
  } else if (modal?.kind === "cancel") {
    modalContent = (
      <CancelModal
        match={modal.match}
        isBusy={isBusy}
        error={modalError}
        onConfirm={() => cancelMutation.mutate(modal.match.id)}
        onClose={closeModal}
      />
    );
  } else if (modal) {
    modalContent = (
      <ResultModal
        match={modal.match}
        form={resultForm}
        onFormChange={setResultForm}
        isBusy={isBusy}
        error={modalError}
        onSubmit={submitResult}
        onClose={closeModal}
        mode={resultMode}
      />
    );
  }

  const isSyncing = isLoading || dailySyncMutation.isPending || syncCooldown;

  return (
    <div className="space-y-6">
      {/* Header with inline date range for sync */}
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-2xl font-bold text-white">Gestión de Partidos</h1>
          <p className="text-sm text-white/50 mt-1">
            {matches.length} partidos en total
          </p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <input
            type="date"
            value={syncStartDate}
            onChange={(e) => setSyncStartDate(e.target.value)}
            title="Fecha inicio (opcional)"
            className="bg-white/5 border border-white/10 rounded-lg px-2 py-1.5 text-xs text-white/70 focus:outline-none focus:ring-1 focus:ring-blue-500/50"
          />
          <span className="text-white/30 text-xs">→</span>
          <input
            type="date"
            value={syncEndDate}
            onChange={(e) => setSyncEndDate(e.target.value)}
            title="Fecha fin (opcional)"
            className="bg-white/5 border border-white/10 rounded-lg px-2 py-1.5 text-xs text-white/70 focus:outline-none focus:ring-1 focus:ring-blue-500/50"
          />
          <button
            onClick={handleSync}
            disabled={isSyncing}
            className="flex items-center gap-2 px-3 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white text-sm transition-colors disabled:opacity-50"
          >
            <RefreshCw className={cn("h-4 w-4", isSyncing && "animate-spin")} />
            Actualizar
          </button>
        </div>
      </div>

      {syncResult && (
        <div className="p-3 rounded-lg bg-green-500/10 border border-green-500/20 text-green-300 text-xs flex items-center gap-2">
          <CheckCircle className="h-4 w-4 shrink-0" />
          <span>
            Sync completado — vinculados: <strong>{syncResult.linked}</strong>,
            kickoffs corregidos: <strong>{syncResult.kickoffs_updated}</strong>,
            marcadores corregidos:{" "}
            <strong>{syncResult.scores_corrected}</strong>
          </span>
        </div>
      )}
      {syncError && (
        <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-300 text-xs flex items-center gap-2">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{syncError}</span>
        </div>
      )}

      <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20 text-blue-300 text-xs">
        Los cambios de estado se aplican manualmente desde aquí como respaldo al
        sincronizador automático. El worker de partidos (cada 30 s) puede
        sobreescribir estos estados si está activo.
      </div>

      {/* Tabs */}
      <AdminTabBar
        tabs={TABS}
        activeTab={tab}
        counts={counts}
        onTabChange={changeTab}
        activeButtonClass="bg-blue-500/20 text-blue-400 ring-1 ring-blue-500/40"
        activeBadgeClass="bg-blue-500/30 text-blue-300"
      />

      {/* Content */}
      {tab === "bracket" ? (
        <BracketTab getToken={getToken} />
      ) : (
        <AdminContentState
          isLoading={isLoading}
          error={error}
          isEmpty={filtered.length === 0}
          emptyTitle="Sin partidos"
          emptyMessage={emptyMsg}
          errorMessage="Error al cargar los partidos."
        >
          <div className="space-y-3">
            <div className="overflow-x-auto rounded-xl border border-white/10">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/10 bg-white/5 text-white/50 text-left">
                    <th className="px-4 py-3 font-medium">#</th>
                    <th className="px-4 py-3 font-medium">Partido</th>
                    <th className="px-4 py-3 font-medium">Fase</th>
                    <th className="px-4 py-3 font-medium">Kickoff</th>
                    <th className="px-4 py-3 font-medium">Marcador</th>
                    <th className="px-4 py-3 font-medium">Estado</th>
                    <th className="px-4 py-3 font-medium text-right">
                      Acciones
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {paginated.map((match) => (
                    <tr
                      key={match.id}
                      className={cn(
                        "border-b border-white/5 hover:bg-white/[0.03] transition-colors",
                        match.status === "in_progress" && "bg-green-500/[0.03]",
                      )}
                    >
                      <td className="px-4 py-3 text-white/40 font-mono text-xs">
                        #{match.id}
                        {match.external_match_id ? (
                          <span
                            className="ml-1 text-green-400/70"
                            title={`API-Football ID: ${match.external_match_id}`}
                          >
                            ●
                          </span>
                        ) : (
                          <span
                            className="ml-1 text-red-400/50"
                            title="Sin vincular a API-Football"
                          >
                            ○
                          </span>
                        )}
                      </td>

                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-white">
                            {match.home_team}
                          </span>
                          <span className="text-white/30">vs</span>
                          <span className="font-medium text-white">
                            {match.away_team}
                          </span>
                        </div>
                        {match.stadium && (
                          <p className="text-white/30 text-xs mt-0.5">
                            {match.stadium.name}
                          </p>
                        )}
                      </td>

                      <td className="px-4 py-3 text-white/60 text-xs">
                        <div>{phaseLabel(match.phase)}</div>
                        {match.group_label && (
                          <div className="text-white/30 mt-0.5">
                            Grupo {match.group_label}
                          </div>
                        )}
                      </td>

                      <td
                        className="px-4 py-3 text-white/60 text-xs"
                        title={
                          match.kickoff_at
                            ? formatDateTime(match.kickoff_at)
                            : undefined
                        }
                      >
                        {match.kickoff_at
                          ? formatRelative(match.kickoff_at)
                          : "—"}
                      </td>

                      <td className="px-4 py-3 font-bold text-white tabular-nums">
                        {scoreLabel(match)}
                        {match.win_method && match.win_method !== "normal" && (
                          <span className="ml-1 text-white/30 font-normal text-xs">
                            ({match.win_method === "extra_time" ? "ET" : "PEN"})
                          </span>
                        )}
                      </td>

                      <td className="px-4 py-3">
                        <StatusBadge status={match.status} />
                      </td>

                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          {match.status === "scheduled" && (
                            <button
                              onClick={() => openStart(match)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-green-500/15 hover:bg-green-500/25 text-green-400 text-xs font-medium transition-colors"
                            >
                              <Play className="h-3.5 w-3.5" />
                              Iniciar
                            </button>
                          )}
                          {match.status === "in_progress" && (
                            <button
                              onClick={() => openResult(match)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-blue-500/15 hover:bg-blue-500/25 text-blue-400 text-xs font-medium transition-colors"
                            >
                              <Edit3 className="h-3.5 w-3.5" />
                              Resultado
                            </button>
                          )}
                          {match.status === "finished" && (
                            <button
                              onClick={() => openCorrect(match)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-white/5 hover:bg-white/10 text-white/50 text-xs font-medium transition-colors"
                            >
                              <Edit3 className="h-3.5 w-3.5" />
                              Corregir
                            </button>
                          )}
                          {(match.status === "scheduled" ||
                            match.status === "in_progress") && (
                            <button
                              onClick={() => openCancel(match)}
                              className="flex items-center gap-1 px-2.5 py-1 rounded-md bg-red-500/10 hover:bg-red-500/20 text-red-400 text-xs font-medium transition-colors"
                            >
                              <Ban className="h-3.5 w-3.5" />
                              Cancelar
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {tab === "all" && totalPages > 1 && (
              <div className="flex items-center justify-between px-1">
                <p className="text-xs text-white/40">
                  {(page - 1) * PAGE_SIZE + 1}–
                  {Math.min(page * PAGE_SIZE, filtered.length)} de{" "}
                  {filtered.length} partidos
                </p>
                <div className="flex items-center gap-1">
                  <button
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page === 1}
                    className="p-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-white/60 hover:text-white transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                  >
                    <ChevronLeft className="h-4 w-4" />
                  </button>
                  <span className="px-3 py-1 text-xs text-white/60 tabular-nums">
                    {page} / {totalPages}
                  </span>
                  <button
                    onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                    disabled={page === totalPages}
                    className="p-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-white/60 hover:text-white transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                  >
                    <ChevronRight className="h-4 w-4" />
                  </button>
                </div>
              </div>
            )}
          </div>
        </AdminContentState>
      )}

      {modal && (
        <AdminModalOverlay onClose={closeModal}>
          {modalContent}
        </AdminModalOverlay>
      )}
    </div>
  );
}
