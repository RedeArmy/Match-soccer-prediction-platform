"use client";

import { type ReactNode, useEffect, useMemo, useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CalendarClock,
  CheckCircle2,
  MapPin,
  Save,
  SlidersHorizontal,
  Target,
  Timer,
} from "lucide-react";
import { api } from "@/lib/api";
import type { MatchResponse, PredictionResponse } from "@/lib/api-types";
import { isPhaseVisible } from "@/lib/feature-flags";
import { cn } from "@/lib/utils";
import { LoadingState } from "@/components/shared/LoadingState";
import { EmptyState } from "@/components/shared/EmptyState";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { useI18n } from "@/lib/i18n";

type DraftScores = Record<number, { home: number; away: number }>;
type Filter = "all" | "pending" | "saved" | "past";
type ViewMode = "by-group" | "by-day";
type GroupLabel =
  | "A"
  | "B"
  | "C"
  | "D"
  | "E"
  | "F"
  | "G"
  | "H"
  | "I"
  | "J"
  | "K"
  | "L";

const GROUPS: GroupLabel[] = [
  "A",
  "B",
  "C",
  "D",
  "E",
  "F",
  "G",
  "H",
  "I",
  "J",
  "K",
  "L",
];

function getEmptyState(params: {
  noTodayMatches: boolean;
  filter: Filter;
  filterHides: boolean;
  isToday: boolean;
  t: (key: string) => string;
}): { title: string; desc: string } {
  const { noTodayMatches, filter, filterHides, isToday, t } = params;
  if (noTodayMatches) {
    return {
      title: t("predictions.noMatchesToday"),
      desc: t("predictions.noMatchesTodayDesc"),
    };
  }
  if (filter === "past") {
    return {
      title: t("predictions.noPastMatches"),
      desc: t("predictions.noPastMatchesDesc"),
    };
  }
  if (filterHides && isToday && filter === "pending") {
    return {
      title: t("predictions.allSavedToday"),
      desc: t("predictions.allSavedTodayDesc"),
    };
  }
  if (filterHides && isToday && filter === "saved") {
    return {
      title: t("predictions.noPredictionsToday"),
      desc: t("predictions.noPredictionsTodayDesc"),
    };
  }
  return {
    title: t("predictions.noMatches"),
    desc: t("predictions.noMatchesDesc"),
  };
}

export function PredictionPanel() {
  const { getToken } = useAuth();
  const queryClient = useQueryClient();
  const { t, timeZone } = useI18n();
  const [filter, setFilter] = useState<Filter>("all");
  const [selectedGroup, setSelectedGroup] = useState<GroupLabel>("A");
  const [viewMode, setViewMode] = useState<ViewMode>("by-group");
  const [drafts, setDrafts] = useState<DraftScores>({});
  const [feedback, setFeedback] = useState<{
    type: "success" | "error";
    message: string;
  } | null>(null);

  const matchesQuery = useQuery({
    queryKey: ["matches"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMatches(token!);
    },
    // Re-fetch when the user returns to this tab — the global default is false
    // but live-match state can change while the user is away.
    refetchOnWindowFocus: true,
    refetchInterval: (query) => {
      if (query.state.status === "error") return false;
      const matches = query.state.data ?? [];
      if (matches.some((m) => m.status === "in_progress")) return 30_000;
      const now = Date.now();
      // A scheduled match whose kickoff has already passed is awaiting the
      // sync worker to mark it in_progress.
      const hasPendingSync = matches.some(
        (m) =>
          m.status === "scheduled" &&
          m.kickoff_at != null &&
          new Date(m.kickoff_at).getTime() <= now,
      );
      if (hasPendingSync) return 30_000;
      // A match kicking off within the backend prematch window (10 min) —
      // start polling fast now so the UI transitions the moment the worker
      // marks it in_progress, instead of waiting up to 120 s.
      const hasImminent = matches.some(
        (m) =>
          m.status === "scheduled" &&
          m.kickoff_at != null &&
          new Date(m.kickoff_at).getTime() <= now + 10 * 60 * 1000,
      );
      return hasImminent ? 30_000 : 120_000;
    },
  });

  const predictionsQuery = useQuery({
    queryKey: ["my-predictions"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMyPredictions(token!);
    },
  });

  const systemClockQuery = useQuery({
    queryKey: ["system-clock"],
    queryFn: async () => {
      const res = await fetch("/api/v1/system/clock");
      if (!res.ok) return null;
      const data = (await res.json()) as { now: string };
      // fetchedAt is captured here (at network completion) so the offset
      // between server time and browser time is as accurate as possible.
      // Including it in the return value ensures TanStack Query detects a
      // change on every refetch even when the server returns the same frozen
      // system.date string, which forces serverOffsetMs to be recomputed.
      return { now: data.now, fetchedAt: Date.now() };
    },
    staleTime: 5_000,
    refetchInterval: 30_000,
  });

  // serverOffsetMs = (server time at fetch) - (browser time at fetch).
  // Adding this to Date.now() anywhere gives the virtual "system time",
  // honouring system.date in dev. In production the offset is ~0 ms.
  // Using data.fetchedAt (not Date.now() at render) gives a stable reference
  // that doesn't drift between re-renders; refetchInterval keeps it fresh.
  const serverOffsetMs = useMemo(() => {
    if (!systemClockQuery.data) return 0;
    return (
      new Date(systemClockQuery.data.now).getTime() -
      systemClockQuery.data.fetchedAt
    );
  }, [systemClockQuery.data]);

  const predictionByMatch = useMemo(() => {
    const map = new Map<number, PredictionResponse>();
    for (const prediction of predictionsQuery.data ?? []) {
      map.set(prediction.match_id, prediction);
    }
    return map;
  }, [predictionsQuery.data]);

  // todayStr is derived synchronously so the "Hoy" filter is correct on the
  // very first render (no empty-string flash waiting for useEffect to run).
  const todayStr = useMemo(() => {
    const base = systemClockQuery.data
      ? new Date(systemClockQuery.data.now)
      : new Date();
    return base.toLocaleDateString("sv", { timeZone });
  }, [systemClockQuery.data, timeZone]);

  useEffect(() => {
    if (!feedback) return;
    const timer = setTimeout(() => setFeedback(null), 3000);
    return () => clearTimeout(timer);
  }, [feedback]);

  useEffect(() => {
    setDrafts((current) => {
      const next = { ...current };
      for (const prediction of predictionsQuery.data ?? []) {
        if (!next[prediction.match_id]) {
          next[prediction.match_id] = {
            home: prediction.home_score,
            away: prediction.away_score,
          };
        }
      }
      return next;
    });
  }, [predictionsQuery.data]);

  const mutation = useMutation({
    mutationFn: async ({
      match,
      draft,
    }: {
      match: MatchResponse;
      draft: { home: number; away: number };
    }) => {
      const token = await getToken();
      const existing = predictionByMatch.get(match.id);
      if (existing) {
        return api.updatePrediction(token!, existing.id, {
          home_score: draft.home,
          away_score: draft.away,
        });
      }
      return api.submitPrediction(token!, {
        match_id: match.id,
        home_score: draft.home,
        away_score: draft.away,
      });
    },
    onSuccess: async () => {
      setFeedback({ type: "success", message: t("predictions.success") });
      await queryClient.invalidateQueries({ queryKey: ["my-predictions"] });
    },
    onError: () => {
      setFeedback({ type: "error", message: t("predictions.error") });
    },
  });

  const sortedMatches = useMemo(() => {
    const ts = (s: string | null) => (s ? new Date(s).getTime() : Infinity);
    const liveFirst = (status: string) => (status === "in_progress" ? 0 : 1);
    return [...(matchesQuery.data ?? [])]
      .filter((m) => isPhaseVisible(m.phase))
      .sort((a, b) => {
        const liveDiff = liveFirst(a.status) - liveFirst(b.status);
        if (liveDiff !== 0) return liveDiff;
        return ts(a.kickoff_at) - ts(b.kickoff_at);
      });
  }, [matchesQuery.data]);

  const isToday = viewMode === "by-day";

  const baseMatches = isToday
    ? sortedMatches.filter((match) => {
        if (!match.kickoff_at) return false;
        return (
          new Date(match.kickoff_at).toLocaleDateString("sv", { timeZone }) ===
          todayStr
        );
      })
    : sortedMatches.filter(
        (match) => normalizeGroup(match.group_label) === selectedGroup,
      );

  const visibleMatches = baseMatches.filter((match) => {
    const hasPrediction = predictionByMatch.has(match.id);
    const isFinished =
      match.status === "finished" || match.status === "cancelled";
    if (filter === "pending") return !isFinished && !hasPrediction;
    if (filter === "saved") return !isFinished && hasPrediction;
    if (filter === "past") return isFinished;
    return true;
  });

  const isLoading = matchesQuery.isLoading || predictionsQuery.isLoading;
  const isError = matchesQuery.isError || predictionsQuery.isError;

  const noTodayMatches = isToday && baseMatches.length === 0;
  const filterHides = baseMatches.length > 0 && visibleMatches.length === 0;

  const { title: emptyTitle, desc: emptyDesc } = getEmptyState({
    noTodayMatches,
    filter,
    filterHides,
    isToday,
    t,
  });

  function updateDraft(matchId: number, value: { home: number; away: number }) {
    setDrafts((current) => ({ ...current, [matchId]: value }));
  }

  function renderContent() {
    if (isLoading) return <LoadingState rows={4} />;
    if (isError) {
      return (
        <EmptyState
          title={t("predictions.loadError")}
          description={t("predictions.loadErrorDesc")}
          icon={<Target className="h-10 w-10" />}
        />
      );
    }
    if (visibleMatches.length === 0) {
      return (
        <EmptyState
          title={emptyTitle}
          description={emptyDesc}
          icon={<Target className="h-10 w-10" />}
        />
      );
    }
    return (
      <div className="grid gap-3">
        {visibleMatches.map((match) => {
          const prediction = predictionByMatch.get(match.id);
          const draft = drafts[match.id] ?? {
            home: prediction?.home_score ?? 0,
            away: prediction?.away_score ?? 0,
          };
          return (
            <PredictionMatchCard
              key={match.id}
              match={match}
              prediction={prediction}
              draft={draft}
              isPending={
                mutation.isPending && mutation.variables?.match.id === match.id
              }
              serverOffsetMs={serverOffsetMs}
              onDraftChange={(value) => updateDraft(match.id, value)}
              onSave={() => mutation.mutate({ match, draft })}
            />
          );
        })}
      </div>
    );
  }

  return (
    <section className="panel overflow-hidden">
      <div className="wc26-stripe" />
      <div className="p-4 sm:p-5">
        <div className="mb-5 flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div>
            <div className="mb-2 flex items-center gap-2">
              <Target className="h-5 w-5 text-green-300" />
              <h2 className="text-lg font-semibold text-white">
                {t("predictions.title")}
              </h2>
            </div>
            <p className="max-w-2xl text-sm text-text-secondary">
              {t("predictions.subtitle")}
            </p>
          </div>

          <div className="flex shrink-0 items-center gap-2 rounded-xl border border-white/10 bg-white/[0.035] p-1">
            <SlidersHorizontal className="ml-2 h-4 w-4 text-text-muted" />
            {(
              [
                ["all", t("predictions.filterAll")],
                ["pending", t("predictions.filterPending")],
                ["saved", t("predictions.filterSaved")],
                ["past", t("predictions.filterPast")],
              ] as const
            ).map(([key, label]) => (
              <button
                key={key}
                type="button"
                onClick={() => setFilter(key)}
                className={cn(
                  "rounded px-2.5 py-1.5 text-xs font-medium transition-colors",
                  filter === key
                    ? "bg-gold-400 text-blue-950"
                    : "text-text-muted hover:text-text-primary",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        <div className="mb-4 inline-flex gap-1 rounded-xl border border-white/10 bg-white/[0.035] p-1">
          {(
            [
              ["by-group", t("predictions.viewByGroup")],
              ["by-day", t("predictions.viewByDay")],
            ] as const
          ).map(([key, label]) => (
            <button
              key={key}
              type="button"
              onClick={() => setViewMode(key)}
              className={cn(
                "rounded px-3 py-1.5 text-xs font-semibold uppercase tracking-wide transition-colors",
                viewMode === key
                  ? "bg-gold-400 text-blue-950"
                  : "text-text-muted hover:text-text-primary",
              )}
            >
              {label}
            </button>
          ))}
        </div>

        {viewMode === "by-group" && (
          <div className="mb-5 rounded-2xl border border-white/10 bg-[#07111F] p-3 sm:p-4">
            <div className="grid grid-cols-6 gap-2 lg:grid-cols-12">
              {GROUPS.map((group) => (
                <GroupButton
                  key={group}
                  label={group}
                  active={selectedGroup === group}
                  onClick={() => setSelectedGroup(group)}
                />
              ))}
            </div>
          </div>
        )}

        {feedback && (
          <div
            className={cn(
              "mb-4 rounded border px-3 py-2 text-sm",
              feedback.type === "success"
                ? "border-green-400/25 bg-green-400/10 text-green-200"
                : "border-red-400/25 bg-red-400/10 text-red-200",
            )}
          >
            {feedback.message}
          </div>
        )}

        {renderContent()}

        <p className="mt-4 text-xs text-text-muted">
          {t("predictions.exactHint")}
        </p>
      </div>
    </section>
  );
}

// ── Per-match card ─────────────────────────────────────────────────────────────

function getButtonLabel(
  isPending: boolean,
  hasPrediction: boolean,
  t: (key: string) => string,
): string {
  if (isPending) return t("common.saving");
  if (hasPrediction) return t("predictions.update");
  return t("predictions.submit");
}

interface PredictionMatchCardProps {
  readonly match: MatchResponse;
  readonly prediction: PredictionResponse | undefined;
  readonly draft: { home: number; away: number };
  readonly isPending: boolean;
  readonly serverOffsetMs: number;
  readonly onDraftChange: (value: { home: number; away: number }) => void;
  readonly onSave: () => void;
}

function PredictionMatchCard({
  match,
  prediction,
  draft,
  isPending,
  serverOffsetMs,
  onDraftChange,
  onSave,
}: PredictionMatchCardProps) {
  const { t, teamName, formatKickoff, phaseName } = useI18n();

  // Virtual clock — single interval drives background colour, lock state, and countdown.
  const [virtualNow, setVirtualNow] = useState(
    () => Date.now() + serverOffsetMs,
  );
  useEffect(() => {
    const id = setInterval(
      () => setVirtualNow(Date.now() + serverOffsetMs),
      1_000,
    );
    return () => clearInterval(id);
  }, [serverOffsetMs]);

  // DB status is the authoritative state once the sync worker has caught up.
  const isLive = match.status === "in_progress";
  const isFinished =
    match.status === "finished" || match.status === "cancelled";

  // Client-side kickoff guard: lock predictions the moment the countdown
  // reaches zero, before the sync worker updates the DB status. This closes
  // the gap between kickoff time and the first PollAndApply cycle.
  const kickoffMs = match.kickoff_at ? new Date(match.kickoff_at).getTime() : null;
  const isKickoffPassed = kickoffMs !== null && virtualNow >= kickoffMs;

  // Visible to the user as an amber "Iniciando..." badge while the worker
  // transitions the DB status from scheduled → in_progress.
  const isPendingSync = isKickoffPassed && !isLive && !isFinished;

  const locked = isLive || isFinished || isKickoffPassed;

  const buttonLabel = getButtonLabel(isPending, prediction !== undefined, t);

  let articleClass = "border-white/10 bg-white/[0.025]";
  if (isFinished) articleClass = "border-red-500/30 bg-red-500/[0.04]";
  else if (isLive) articleClass = "border-green-500/30 bg-green-500/[0.04]";
  else if (isPendingSync) articleClass = "border-amber-500/30 bg-amber-500/[0.04]";

  let statusBadge: ReactNode;
  if (isLive) {
    statusBadge = (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-green-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-green-300">
        <span className="relative flex h-1.5 w-1.5 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green-400" />
        </span>
        {t("predictions.liveLabel")}
      </span>
    );
  } else if (isPendingSync) {
    statusBadge = (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-amber-300">
        <span className="relative flex h-1.5 w-1.5 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-amber-400 opacity-75" />
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-amber-400" />
        </span>
        {t("predictions.pendingSync")}
      </span>
    );
  } else {
    statusBadge = <StatusBadge status={match.status} size="sm" />;
  }

  return (
    <article
      className={cn("rounded border p-4 transition-colors", articleClass)}
    >
      <div className="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-center">
        <div className="min-w-0">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            {statusBadge}
            {!isFinished && (
              <span
                className={cn(
                  "inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[10px] font-medium",
                  prediction
                    ? "border-green-400/30 bg-green-400/10 text-green-200"
                    : "border-gold-400/25 bg-gold-400/10 text-gold-200",
                )}
              >
                {prediction ? (
                  <CheckCircle2 className="h-3 w-3" />
                ) : (
                  <Target className="h-3 w-3" />
                )}
                {prediction ? t("predictions.saved") : t("predictions.unsaved")}
              </span>
            )}
          </div>

          <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-3">
            <TeamLabel label={teamName(match.home_team)} align="right" />
            {isLive || isFinished ? (
              <span className="font-score text-lg font-bold tabular-nums text-white">
                {match.home_score ?? 0}&nbsp;–&nbsp;{match.away_score ?? 0}
              </span>
            ) : (
              <span className="font-score text-xs text-text-muted">vs</span>
            )}
            <TeamLabel label={teamName(match.away_team)} align="left" />
          </div>

          <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-muted">
            <span className="inline-flex items-center gap-1.5">
              <CalendarClock className="h-3.5 w-3.5" />
              {t("predictions.kickoff")}:{" "}
              <span suppressHydrationWarning>
                {formatKickoff(match.kickoff_at)}
              </span>
            </span>
            {match.stadium && (
              <span className="inline-flex items-center gap-1.5">
                <MapPin className="h-3.5 w-3.5" />
                {match.stadium.name}
              </span>
            )}
            {(match.phase || match.group_label) && (
              <span>
                {t("predictions.phase")}:{" "}
                {phaseName(match.phase ?? match.group_label)}
              </span>
            )}
            {!isLive && !isFinished && (
              <MatchCountdown
                kickoffAt={match.kickoff_at}
                virtualNow={virtualNow}
              />
            )}
          </div>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <ScoreInput
            label={t("predictions.home")}
            value={draft.home}
            disabled={locked}
            onChange={(value) => onDraftChange({ ...draft, home: value })}
          />
          <ScoreInput
            label={t("predictions.away")}
            value={draft.away}
            disabled={locked}
            onChange={(value) => onDraftChange({ ...draft, away: value })}
          />
          {isFinished ? (
            <div className="flex w-full flex-col items-center justify-center gap-0.5 rounded-lg border border-gold-400/20 bg-gold-400/10 px-4 py-2 sm:w-auto sm:min-w-[4.5rem]">
              <span className="text-[10px] uppercase tracking-wide text-text-muted">
                {t("predictions.points")}
              </span>
              <span className="font-score text-2xl font-bold tabular-nums text-gold-300">
                {prediction?.points ?? "–"}
              </span>
            </div>
          ) : (
            <button
              type="button"
              disabled={locked || isPending}
              onClick={onSave}
              className="btn-gold w-full px-3 py-2 text-sm sm:w-auto disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Save className="h-4 w-4 shrink-0" />
              <span className="relative">
                <span aria-hidden className="invisible">
                  {t("predictions.submit")}
                </span>
                <span className="absolute inset-0 flex items-center justify-center">
                  {buttonLabel}
                </span>
              </span>
            </button>
          )}
        </div>
      </div>
    </article>
  );
}

// ── Shared sub-components ──────────────────────────────────────────────────────

function normalizeGroup(group: string | null | undefined): GroupLabel | null {
  const value = group
    ?.trim()
    .toUpperCase()
    .replace(/^GROUP\s+/, "")
    .replace(/^GRUPO\s+/, "");
  return GROUPS.includes(value as GroupLabel) ? (value as GroupLabel) : null;
}

function GroupButton({
  label,
  active,
  onClick,
}: Readonly<{
  label: string;
  active: boolean;
  onClick: () => void;
}>) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex min-h-14 flex-col items-center justify-center rounded-xl border px-2 py-2 text-center transition-colors",
        active
          ? "border-gold-400 bg-gold-400 text-blue-950 shadow-lg shadow-gold-400/10"
          : "border-white/10 bg-white/[0.035] text-text-secondary hover:border-gold-400/40 hover:text-white",
      )}
    >
      <span className="text-sm font-bold uppercase">{label}</span>
    </button>
  );
}

function MatchCountdown({
  kickoffAt,
  virtualNow,
}: Readonly<{ kickoffAt: string | null | undefined; virtualNow: number }>) {
  if (!kickoffAt) return null;
  const diff = new Date(kickoffAt).getTime() - virtualNow;
  if (diff <= 0) return null;

  const days = Math.floor(diff / 86_400_000);
  const hours = Math.floor((diff % 86_400_000) / 3_600_000);
  const mins = Math.floor((diff % 3_600_000) / 60_000);
  const secs = Math.floor((diff % 60_000) / 1_000);

  let label: string;
  if (days > 0) {
    label = `${days}d ${hours}h ${mins}m`;
  } else if (hours > 0) {
    label = `${hours}h ${mins}m ${secs}s`;
  } else {
    label = `${mins}m ${secs}s`;
  }

  return (
    <span className="inline-flex items-center gap-1 tabular-nums text-gold-300">
      <Timer className="h-3 w-3 shrink-0" />
      {label}
    </span>
  );
}

function TeamLabel({
  label,
  align,
}: Readonly<{ label: string; align: "left" | "right" }>) {
  return (
    <div
      className={cn("min-w-0", align === "right" ? "text-right" : "text-left")}
    >
      <p className="truncate text-base font-semibold text-white">{label}</p>
    </div>
  );
}

function ScoreInput({
  label,
  value,
  disabled,
  onChange,
}: Readonly<{
  label: string;
  value: number;
  disabled: boolean;
  onChange: (value: number) => void;
}>) {
  return (
    <label className="block w-full sm:w-20">
      <span className="mb-1 block text-[10px] uppercase text-text-muted">
        {label}
      </span>
      <input
        type="number"
        min={0}
        max={50}
        inputMode="numeric"
        disabled={disabled}
        value={value}
        onFocus={(event) => event.target.select()}
        onKeyDown={(event) => {
          if (
            !/[\d\b]/.test(event.key) &&
            !["Backspace", "Delete", "ArrowLeft", "ArrowRight", "Tab"].includes(
              event.key,
            )
          ) {
            event.preventDefault();
          }
        }}
        onChange={(event) => {
          const raw = Number(event.target.value);
          if (!Number.isFinite(raw)) return;
          onChange(Math.min(50, Math.max(0, raw)));
        }}
        className="input-base h-10 text-center font-score text-lg disabled:cursor-not-allowed disabled:opacity-55"
      />
    </label>
  );
}
