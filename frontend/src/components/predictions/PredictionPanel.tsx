"use client";

import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Calendar,
  CalendarClock,
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  MapPin,
  Save,
  SlidersHorizontal,
  Target,
  Timer,
} from "lucide-react";
import { api } from "@/lib/api";
import type {
  MatchResponse,
  PredictionResponse,
  ExtraPredictionResponse,
  ExtraType,
} from "@/lib/api-types";
import {
  isKnockoutPlaceholder,
  visibleKnockoutPhases,
} from "@/lib/feature-flags";
import { useSlots } from "@/hooks/useSlots";
import { cn } from "@/lib/utils";
import { LoadingState } from "@/components/shared/LoadingState";
import { EmptyState } from "@/components/shared/EmptyState";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { type Locale, useI18n } from "@/lib/i18n";

type DraftScores = Record<
  number,
  {
    home: number;
    away: number;
    winMethod?: string | null; // "extra_time" | "penalties" | null
    penaltyWinner?: string | null; // "home" | "away" | null
  }
>;
type Filter = "all" | "pending" | "saved" | "past";
const PAGE_SIZE = 6;
type GroupLabel =
  "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J" | "K" | "L";

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
  isByDay: boolean;
  noMatchesInRange: boolean;
  isExactlyToday: boolean;
  isRange: boolean;
  filter: Filter;
  filterHides: boolean;
  t: (key: string) => string;
}): { title: string; desc: string } {
  const {
    isByDay,
    noMatchesInRange,
    isExactlyToday,
    isRange,
    filter,
    filterHides,
    t,
  } = params;
  if (noMatchesInRange) {
    if (isExactlyToday) {
      return {
        title: t("predictions.noMatchesToday"),
        desc: t("predictions.noMatchesTodayDesc"),
      };
    }
    if (isRange) {
      return {
        title: t("predictions.noMatchesForRange"),
        desc: t("predictions.noMatchesForRangeDesc"),
      };
    }
    return {
      title: t("predictions.noMatchesForDate"),
      desc: t("predictions.noMatchesForDateDesc"),
    };
  }
  if (filter === "past") {
    return {
      title: t("predictions.noPastMatches"),
      desc: t("predictions.noPastMatchesDesc"),
    };
  }
  if (filterHides && isByDay && isExactlyToday && filter === "pending") {
    return {
      title: t("predictions.allSavedToday"),
      desc: t("predictions.allSavedTodayDesc"),
    };
  }
  if (filterHides && isByDay && isExactlyToday && filter === "saved") {
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

const PHASE_KEY_MAP: Record<string, string> = {
  round_of_32: "phaseRoundOf32",
  round_of_16: "phaseRoundOf16",
  quarter_final: "phaseQuarterFinal",
  semi_final: "phaseSemiF",
  third_place: "phaseThirdPlace",
  final: "phaseFinal",
};

function phaseShortLabel(phase: string, t: (key: string) => string): string {
  const key = PHASE_KEY_MAP[phase];
  return key ? t(key) : phase;
}

// Returns the earliest knockout phase that still has non-finished matches, so
// the panel defaults to the currently active round instead of always showing
// group stage. Falls back to the most recent completed phase when all rounds
// are finished (knockoutPhases is ordered latest-first per KNOCKOUT_TAB_ORDER).
function activeKnockoutPhase(
  matches: { phase: string | null; status: string }[],
  knockoutPhases: string[],
): string | null {
  if (knockoutPhases.length === 0) return null;
  for (const phase of [...knockoutPhases].reverse()) {
    if (
      matches.some(
        (m) =>
          m.phase === phase &&
          m.status !== "finished" &&
          m.status !== "cancelled",
      )
    )
      return phase;
  }
  return knockoutPhases[0];
}

export function PredictionPanel() {
  const { getToken } = useAuth();
  const queryClient = useQueryClient();
  const { t, timeZone, locale } = useI18n();
  const { teamByAutoSource } = useSlots();
  const [filter, setFilter] = useState<Filter>("all");
  const [selectedGroup, setSelectedGroup] = useState<GroupLabel>("A");
  const [viewMode, setViewMode] = useState<string>("by-group");
  const hasAutoSwitched = useRef(false);
  const [drafts, setDrafts] = useState<DraftScores>({});
  const [feedback, setFeedback] = useState<{
    type: "success" | "error";
    message: string;
  } | null>(null);
  // "" means "not yet user-set" — falls back to todayStr after it's computed
  const [calendarStart, setCalendarStart] = useState<string>("");
  const [calendarEnd, setCalendarEnd] = useState<string>("");
  const [page, setPage] = useState(0);

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

  // Extras (bonus predictions) are bulk-fetched for every match currently
  // loaded, avoiding an N+1 request per card.
  const matchIds = useMemo(
    () => (matchesQuery.data ?? []).map((m) => m.id),
    [matchesQuery.data],
  );
  const extrasQuery = useQuery({
    queryKey: ["my-extras", matchIds],
    queryFn: async () => {
      const token = await getToken();
      return api.getMyExtras(token!, matchIds);
    },
    enabled: matchIds.length > 0,
  });

  const extrasByMatch = useMemo(() => {
    const map = new Map<
      number,
      { first_scorer?: ExtraPredictionResponse; halftime_result?: ExtraPredictionResponse }
    >();
    for (const extra of extrasQuery.data ?? []) {
      const entry = map.get(extra.match_id) ?? {};
      if (extra.extra_type === "first_scorer") entry.first_scorer = extra;
      if (extra.extra_type === "halftime_result")
        entry.halftime_result = extra;
      map.set(extra.match_id, entry);
    }
    return map;
  }, [extrasQuery.data]);

  const [extraPending, setExtraPending] = useState<{
    matchId: number;
    extraType: ExtraType;
  } | null>(null);

  const extraMutation = useMutation({
    mutationFn: async ({
      matchId,
      extraType,
      answer,
    }: {
      matchId: number;
      extraType: ExtraType;
      answer: string;
    }) => {
      const token = await getToken();
      return api.submitExtra(token!, {
        match_id: matchId,
        extra_type: extraType,
        answer,
      });
    },
    onMutate: ({ matchId, extraType }) => {
      setExtraPending({ matchId, extraType });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["my-extras"] });
    },
    onError: () => {
      setFeedback({ type: "error", message: t("predictions.extraSaveError") });
    },
    onSettled: () => {
      setExtraPending(null);
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

  // todayStr is derived synchronously so the "Partidos" filter is correct on the
  // very first render (no empty-string flash waiting for useEffect to run).
  const todayStr = useMemo(() => {
    const base = systemClockQuery.data
      ? new Date(systemClockQuery.data.now)
      : new Date();
    return base.toLocaleDateString("sv", { timeZone });
  }, [systemClockQuery.data, timeZone]);

  // Effective calendar range: fall back to today until the user picks dates
  const effectiveStart = calendarStart || todayStr;
  const effectiveEnd = calendarEnd || todayStr;

  useEffect(() => {
    if (!feedback) return;
    const timer = setTimeout(() => setFeedback(null), 3000);
    return () => clearTimeout(timer);
  }, [feedback]);

  useEffect(() => {
    setPage(0);
  }, [filter, effectiveStart, effectiveEnd, viewMode]);

  useEffect(() => {
    setDrafts((current) => {
      const next = { ...current };
      for (const prediction of predictionsQuery.data ?? []) {
        if (!next[prediction.match_id]) {
          next[prediction.match_id] = {
            home: prediction.home_score,
            away: prediction.away_score,
            winMethod: prediction.predicted_win_method ?? null,
            penaltyWinner: prediction.predicted_penalty_winner ?? null,
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
      draft: DraftScores[number];
    }) => {
      const token = await getToken();
      const existing = predictionByMatch.get(match.id);
      const winMethodPayload = draft.winMethod ?? undefined;
      const penaltyWinnerPayload = draft.penaltyWinner ?? undefined;
      if (existing) {
        return api.updatePrediction(token!, existing.id, {
          home_score: draft.home,
          away_score: draft.away,
          predicted_win_method: winMethodPayload,
          predicted_penalty_winner: penaltyWinnerPayload,
        });
      }
      return api.submitPrediction(token!, {
        match_id: match.id,
        home_score: draft.home,
        away_score: draft.away,
        predicted_win_method: winMethodPayload,
        predicted_penalty_winner: penaltyWinnerPayload,
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
    const tier = (status: string) => {
      if (status === "in_progress") return 0;
      if (status === "finished" || status === "cancelled") return 2;
      return 1;
    };
    return [...(matchesQuery.data ?? [])].sort((a, b) => {
      const tierDiff = tier(a.status) - tier(b.status);
      if (tierDiff !== 0) return tierDiff;
      return ts(a.kickoff_at) - ts(b.kickoff_at);
    });
  }, [matchesQuery.data]);

  // Enrich knockout match team names from confirmed slots when the match record
  // still carries placeholder codes (bridge before the next matches re-fetch).
  const enrichedMatches = useMemo(() => {
    if (teamByAutoSource.size === 0) return sortedMatches;
    return sortedMatches.map((match) => {
      const home =
        isKnockoutPlaceholder(match.home_team) && match.home_team
          ? (teamByAutoSource.get(match.home_team) ?? match.home_team)
          : match.home_team;
      const away =
        isKnockoutPlaceholder(match.away_team) && match.away_team
          ? (teamByAutoSource.get(match.away_team) ?? match.away_team)
          : match.away_team;
      if (home === match.home_team && away === match.away_team) return match;
      return { ...match, home_team: home, away_team: away };
    });
  }, [sortedMatches, teamByAutoSource]);

  const knockoutPhases = useMemo(
    () => visibleKnockoutPhases(enrichedMatches),
    [enrichedMatches],
  );

  // Auto-select the active knockout phase on first data load so users land on
  // the currently relevant round (e.g. round_of_16) instead of group stage
  // once the group phase is over. Runs only once per mount.
  useEffect(() => {
    if (hasAutoSwitched.current || knockoutPhases.length === 0) return;
    const active = activeKnockoutPhase(enrichedMatches, knockoutPhases);
    if (active) {
      setViewMode(active);
      hasAutoSwitched.current = true;
    }
  }, [knockoutPhases, enrichedMatches]);

  // Dates (YYYY-MM-DD) that have at least one match with both teams confirmed —
  // used for calendar dot indicators.
  const matchDates = useMemo(() => {
    const set = new Set<string>();
    for (const match of enrichedMatches) {
      if (!match.kickoff_at) continue;
      if (
        isKnockoutPlaceholder(match.home_team) ||
        isKnockoutPlaceholder(match.away_team)
      )
        continue;
      set.add(
        new Date(match.kickoff_at).toLocaleDateString("sv", { timeZone }),
      );
    }
    return set;
  }, [enrichedMatches, timeZone]);

  const isByDay = viewMode === "by-day";
  const isByGroup = viewMode === "by-group";
  const isKnockoutPhase = !isByDay && !isByGroup;

  let baseMatches;
  if (isByDay) {
    baseMatches = enrichedMatches.filter((match) => {
      if (!match.kickoff_at) return false;
      if (match.status === "finished" || match.status === "cancelled")
        return false;
      if (
        isKnockoutPlaceholder(match.home_team) ||
        isKnockoutPlaceholder(match.away_team)
      )
        return false;
      const matchDate = new Date(match.kickoff_at).toLocaleDateString("sv", {
        timeZone,
      });
      return matchDate >= effectiveStart && matchDate <= effectiveEnd;
    });
  } else if (isKnockoutPhase) {
    baseMatches = enrichedMatches.filter(
      (match) =>
        match.phase === viewMode &&
        !isKnockoutPlaceholder(match.home_team) &&
        !isKnockoutPlaceholder(match.away_team),
    );
  } else {
    baseMatches = enrichedMatches.filter(
      (match) => normalizeGroup(match.group_label) === selectedGroup,
    );
  }

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

  const noMatchesInRange = isByDay && baseMatches.length === 0;
  const isExactlyToday =
    effectiveStart === todayStr && effectiveEnd === todayStr;
  const isRange = effectiveStart !== effectiveEnd;
  const filterHides = baseMatches.length > 0 && visibleMatches.length === 0;

  const { title: emptyTitle, desc: emptyDesc } = getEmptyState({
    isByDay,
    noMatchesInRange,
    isExactlyToday,
    isRange,
    filter,
    filterHides,
    t,
  });

  function updateDraft(matchId: number, value: DraftScores[number]) {
    setDrafts((current) => ({ ...current, [matchId]: value }));
  }

  const totalPages =
    isByDay || isKnockoutPhase
      ? Math.ceil(visibleMatches.length / PAGE_SIZE)
      : 1;
  const pageMatches =
    isByDay || isKnockoutPhase
      ? visibleMatches.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)
      : visibleMatches;

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
      <>
        <div className="grid gap-3">
          {pageMatches.map((match) => {
            const prediction = predictionByMatch.get(match.id);
            const draft = drafts[match.id] ?? {
              home: prediction?.home_score ?? 0,
              away: prediction?.away_score ?? 0,
              winMethod: prediction?.predicted_win_method ?? null,
              penaltyWinner: prediction?.predicted_penalty_winner ?? null,
            };
            const extras = extrasByMatch.get(match.id);
            return (
              <PredictionMatchCard
                key={match.id}
                match={match}
                prediction={prediction}
                draft={draft}
                isPending={
                  mutation.isPending &&
                  mutation.variables?.match.id === match.id
                }
                serverOffsetMs={serverOffsetMs}
                onDraftChange={(value) => updateDraft(match.id, value)}
                onSave={() => mutation.mutate({ match, draft })}
                extraFirstScorer={extras?.first_scorer}
                extraHalftimeResult={extras?.halftime_result}
                extraPendingType={
                  extraPending?.matchId === match.id
                    ? extraPending.extraType
                    : null
                }
                onExtraSubmit={(extraType, answer) =>
                  extraMutation.mutate({ matchId: match.id, extraType, answer })
                }
              />
            );
          })}
        </div>
        {totalPages > 1 && (
          <div className="mt-4 flex items-center justify-center gap-3">
            <button
              type="button"
              onClick={() => setPage((p) => p - 1)}
              disabled={page === 0}
              className={cn(
                "rounded p-1.5 transition-colors",
                page === 0
                  ? "cursor-not-allowed text-text-muted opacity-30"
                  : "text-text-secondary hover:bg-white/10 hover:text-white",
              )}
              aria-label={t("predictions.pagePrev")}
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <span className="text-xs tabular-nums text-text-secondary">
              {page + 1} / {totalPages}
            </span>
            <button
              type="button"
              onClick={() => setPage((p) => p + 1)}
              disabled={page >= totalPages - 1}
              className={cn(
                "rounded p-1.5 transition-colors",
                page >= totalPages - 1
                  ? "cursor-not-allowed text-text-muted opacity-30"
                  : "text-text-secondary hover:bg-white/10 hover:text-white",
              )}
              aria-label={t("predictions.pageNext")}
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        )}
      </>
    );
  }

  return (
    <section className="panel">
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
                ...(isByDay
                  ? []
                  : ([["past", t("predictions.filterPast")]] as const)),
              ] as Array<[Filter, string]>
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

        <div className="mb-4 inline-flex flex-wrap gap-1 rounded-xl border border-white/10 bg-white/[0.035] p-1">
          {knockoutPhases.map((phase) => (
            <button
              key={phase}
              type="button"
              onClick={() => setViewMode(phase)}
              className={cn(
                "rounded px-3 py-1.5 text-xs font-semibold uppercase tracking-wide transition-colors",
                viewMode === phase
                  ? "bg-gold-400 text-blue-950"
                  : "text-text-muted hover:text-text-primary",
              )}
            >
              {phaseShortLabel(phase, t)}
            </button>
          ))}
          {(
            [
              ["by-group", t("predictions.viewByGroup")],
              ["by-day", t("predictions.viewByDay")],
            ] as const
          ).map(([key, label]) => (
            <button
              key={key}
              type="button"
              onClick={() => {
                setViewMode(key);
                if (key === "by-day") {
                  setCalendarStart(todayStr);
                  setCalendarEnd(todayStr);
                  if (filter === "past") setFilter("all");
                }
              }}
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

        {isByGroup && (
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

        {isByDay && (
          <MatchCalendar
            todayStr={todayStr}
            selectedStart={effectiveStart}
            selectedEnd={effectiveEnd}
            matchDates={matchDates}
            locale={locale}
            t={t}
            onRangeChange={(start, end) => {
              setCalendarStart(start);
              setCalendarEnd(end);
            }}
          />
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
  readonly draft: DraftScores[number];
  readonly isPending: boolean;
  readonly serverOffsetMs: number;
  readonly onDraftChange: (value: DraftScores[number]) => void;
  readonly onSave: () => void;
  readonly extraFirstScorer: ExtraPredictionResponse | undefined;
  readonly extraHalftimeResult: ExtraPredictionResponse | undefined;
  readonly extraPendingType: ExtraType | null;
  readonly onExtraSubmit: (extraType: ExtraType, answer: string) => void;
}

function PredictionMatchCard({
  match,
  prediction,
  draft,
  isPending,
  serverOffsetMs,
  extraFirstScorer,
  extraHalftimeResult,
  extraPendingType,
  onExtraSubmit,
  onDraftChange,
  onSave,
}: PredictionMatchCardProps) {
  const { t, teamName, formatKickoff, phaseName } = useI18n();
  const [localError, setLocalError] = useState<string | null>(null);

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

  const isExtraTime = isLive && match.period === "ET";
  const isPenaltiesLive = isLive && match.period === "PEN_LIVE";
  const isFinishedByPenalties = isFinished && match.win_method === "penalties";
  const hasPenaltyScore =
    match.penalty_home_score != null && match.penalty_away_score != null;

  // Client-side kickoff guard: lock predictions the moment the countdown
  // reaches zero, before the sync worker updates the DB status. This closes
  // the gap between kickoff time and the first PollAndApply cycle.
  const kickoffMs = match.kickoff_at
    ? new Date(match.kickoff_at).getTime()
    : null;
  const isKickoffPassed = kickoffMs !== null && virtualNow >= kickoffMs;

  // Visible to the user as an amber "Iniciando..." badge while the worker
  // transitions the DB status from scheduled → in_progress.
  const isPendingSync = isKickoffPassed && !isLive && !isFinished;

  const locked = isLive || isFinished || isKickoffPassed;

  const buttonLabel = getButtonLabel(isPending, prediction !== undefined, t);

  const articleClass = matchCardClass(isFinished, isLive, isPendingSync);
  const isKnockoutUnlocked =
    match.phase !== null && match.phase !== "group_stage" && !locked;
  const statusBadge = matchCardStatusBadge(
    isLive,
    isPendingSync,
    match.status,
    t,
  );

  return (
    <article
      className={cn("rounded border p-4 transition-colors", articleClass)}
    >
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:gap-4">
        {/* ── Match info ── */}
        <div className="min-w-0 flex-1">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            {statusBadge}
            {isExtraTime && (
              <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-amber-300">
                {t("predictions.liveExtraTime")}
              </span>
            )}
            {isPenaltiesLive && (
              <span className="inline-flex items-center gap-1 rounded-full bg-orange-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-orange-300">
                {t("predictions.livePenalties")}
              </span>
            )}
            {isFinishedByPenalties && (
              <span className="inline-flex items-center gap-1 rounded-full bg-orange-500/15 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-orange-300/80">
                {t("predictions.livePenalties")}
              </span>
            )}
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
              <div className="flex flex-col items-center gap-0.5">
                <span className="font-score text-lg font-bold tabular-nums text-white">
                  {match.home_score ?? 0}&nbsp;–&nbsp;{match.away_score ?? 0}
                </span>
                {hasPenaltyScore && (
                  <span className="font-score text-[11px] tabular-nums text-orange-300/80">
                    {t("predictions.penaltiesScoreLabel")}&nbsp;
                    {match.penalty_home_score}&nbsp;–&nbsp;
                    {match.penalty_away_score}
                  </span>
                )}
              </div>
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

        {/* ── Match extras (bonus predictions) — every match ── */}
        <div className="flex shrink-0 flex-col justify-center gap-2 self-stretch rounded border border-white/8 bg-white/[0.03] px-3 py-2 lg:min-w-[11rem]">
          <MatchExtraControl
            label={t("predictions.extraFirstScorer")}
            options={[
              { value: "home", label: teamName(match.home_team) },
              { value: "away", label: teamName(match.away_team) },
              { value: "none", label: t("predictions.extraOptionNone") },
            ]}
            locked={locked}
            isFinished={isFinished}
            prediction={extraFirstScorer}
            resolvedAnswer={match.first_scoring_team ?? null}
            isPending={extraPendingType === "first_scorer"}
            onSubmit={(answer) => onExtraSubmit("first_scorer", answer)}
          />
          <MatchExtraControl
            label={t("predictions.extraHalftimeResult")}
            options={[
              { value: "home", label: teamName(match.home_team) },
              { value: "draw", label: t("predictions.extraOptionDraw") },
              { value: "away", label: teamName(match.away_team) },
            ]}
            locked={locked}
            isFinished={isFinished}
            prediction={extraHalftimeResult}
            resolvedAnswer={halftimeResultAnswer(match)}
            isPending={extraPendingType === "halftime_result"}
            onSubmit={(answer) => onExtraSubmit("halftime_result", answer)}
          />
        </div>

        {/* ── Win-method — knockout phases only, unlocked matches ── */}
        {isKnockoutUnlocked && (
          <div className="flex shrink-0 items-center justify-center self-stretch rounded border border-white/8 bg-white/[0.03] px-3 py-2 lg:min-w-[9rem]">
            <WinMethodSelector
              match={match}
              draft={draft}
              localError={localError}
              onDraftChange={onDraftChange}
              onClearError={() => setLocalError(null)}
            />
          </div>
        )}

        {/* ── Score inputs + action ── */}
        <div className="flex shrink-0 items-center gap-2">
          <ScoreInput
            label={t("predictions.home")}
            value={draft.home}
            disabled={locked}
            onChange={(value) =>
              onDraftChange({
                ...draft,
                home: value,
                winMethod: null,
                penaltyWinner: null,
              })
            }
          />
          <ScoreInput
            label={t("predictions.away")}
            value={draft.away}
            disabled={locked}
            onChange={(value) =>
              onDraftChange({
                ...draft,
                away: value,
                winMethod: null,
                penaltyWinner: null,
              })
            }
          />
          {isFinished ? (
            <div className="flex flex-col items-center justify-center gap-0.5 rounded-lg border border-gold-400/20 bg-gold-400/10 px-4 py-2 min-w-[4.5rem]">
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
              onClick={() => {
                if (
                  isKnockoutUnlocked &&
                  draft.home === draft.away &&
                  !draft.penaltyWinner
                ) {
                  setLocalError(t("predictions.selectPenaltyWinner"));
                  return;
                }
                setLocalError(null);
                onSave();
              }}
              className="btn-gold px-2 py-1.5 text-xs disabled:cursor-not-allowed disabled:opacity-50"
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

// halftimeResultAnswer derives the resolved "home"/"draw"/"away" half-time
// outcome from the match's halftime score fields, or null when either score
// is unresolved (sync worker never supplied it, or the match finished before
// this feature existed).
function halftimeResultAnswer(match: MatchResponse): string | null {
  if (match.halftime_home_score == null || match.halftime_away_score == null)
    return null;
  if (match.halftime_home_score > match.halftime_away_score) return "home";
  if (match.halftime_home_score < match.halftime_away_score) return "away";
  return "draw";
}

interface MatchExtraControlProps {
  readonly label: string;
  readonly options: readonly { value: string; label: string }[];
  readonly locked: boolean;
  readonly isFinished: boolean;
  readonly prediction: ExtraPredictionResponse | undefined;
  readonly resolvedAnswer: string | null;
  readonly isPending: boolean;
  readonly onSubmit: (answer: string) => void;
}

// MatchExtraControl renders one match extra (bonus prediction) as a compact
// dropdown that auto-submits on change — extras are a single independent
// value, unlike the scoreline prediction which batches home/away/win-method
// into one Save action. Once the match is finished it switches to a
// read-only summary of the resolved answer and points earned (or an
// em-dash when the match never resolved this extra).
function MatchExtraControl({
  label,
  options,
  locked,
  isFinished,
  prediction,
  resolvedAnswer,
  isPending,
  onSubmit,
}: MatchExtraControlProps) {
  const { t } = useI18n();

  if (isFinished) {
    const resolvedLabel =
      resolvedAnswer == null
        ? "—"
        : (options.find((o) => o.value === resolvedAnswer)?.label ??
          resolvedAnswer);
    return (
      <div className="flex items-center justify-between gap-2 text-xs">
        <span className="text-text-muted">{label}</span>
        <span className="flex items-center gap-1.5 font-medium text-white">
          {resolvedAnswer == null ? (
            <span className="text-text-muted">
              {t("predictions.extraUnavailable")}
            </span>
          ) : (
            <>
              {resolvedLabel}
              {prediction?.points != null && (
                <span className="text-gold-300">
                  +{prediction.points} {t("predictions.extraPointsEarned")}
                </span>
              )}
            </>
          )}
        </span>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between gap-2 text-xs">
      <span className="text-text-muted">{label}</span>
      <select
        aria-label={label}
        className="rounded border border-white/10 bg-black/20 px-1.5 py-1 text-xs text-white disabled:cursor-not-allowed disabled:opacity-50"
        value={prediction?.answer ?? ""}
        disabled={locked || isPending}
        onChange={(e) => {
          if (e.target.value) onSubmit(e.target.value);
        }}
      >
        <option value="" disabled label={t("predictions.extraPick")} />
        {options.map((opt) => (
          // The label attribute (not text content) supplies the option's
          // display/accessible text. Team names are already rendered
          // elsewhere on this card (TeamLabel, other extra selects); giving
          // these options empty text content avoids duplicate DOM text nodes
          // for the same team name while remaining fully accessible — the
          // HTML spec defines label as exactly this: the text to display
          // when it differs from (or replaces) the element's content.
          <option key={opt.value} value={opt.value} label={opt.label} />
        ))}
      </select>
    </div>
  );
}

// ── Shared sub-components ──────────────────────────────────────────────────────

function matchCardClass(
  isFinished: boolean,
  isLive: boolean,
  isPendingSync: boolean,
): string {
  if (isFinished) return "border-red-500/30 bg-red-500/[0.04]";
  if (isLive) return "border-green-500/30 bg-green-500/[0.04]";
  if (isPendingSync) return "border-amber-500/30 bg-amber-500/[0.04]";
  return "border-white/10 bg-white/[0.025]";
}

function matchCardStatusBadge(
  isLive: boolean,
  isPendingSync: boolean,
  status: string,
  t: (key: string) => string,
): ReactNode {
  if (isLive) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-green-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-green-300">
        <span className="relative flex h-1.5 w-1.5 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green-400" />
        </span>
        {t("predictions.liveLabel")}
      </span>
    );
  }
  if (isPendingSync) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-500/20 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-amber-300">
        <span className="relative flex h-1.5 w-1.5 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-amber-400 opacity-75" />
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-amber-400" />
        </span>
        {t("predictions.pendingSync")}
      </span>
    );
  }
  return <StatusBadge status={status} size="sm" />;
}

interface WinMethodSelectorProps {
  readonly match: MatchResponse;
  readonly draft: DraftScores[number];
  readonly localError: string | null;
  readonly onDraftChange: (value: DraftScores[number]) => void;
  readonly onClearError: () => void;
}

function WinMethodSelector({
  match,
  draft,
  localError,
  onDraftChange,
  onClearError,
}: WinMethodSelectorProps) {
  const { t, teamName } = useI18n();

  if (draft.home === draft.away) {
    return (
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-text-secondary">
          {t("predictions.penaltyWinner")}
        </span>
        <select
          aria-label={t("predictions.penaltyWinner")}
          value={draft.penaltyWinner ?? ""}
          onChange={(e) => {
            onClearError();
            const val = e.target.value as "home" | "away" | "";
            if (val === "") {
              onDraftChange({ ...draft, penaltyWinner: null, winMethod: null });
            } else {
              onDraftChange({
                ...draft,
                penaltyWinner: val,
                winMethod: "penalties",
              });
            }
          }}
          className="w-full rounded border border-white/15 bg-white/5 px-2 py-1.5 text-xs text-white focus:border-gold-400/60 focus:outline-none"
        >
          <option value="" className="bg-[#0D1420]">
            —
          </option>
          <option value="home" className="bg-[#0D1420]">
            {teamName(match.home_team)}
          </option>
          <option value="away" className="bg-[#0D1420]">
            {teamName(match.away_team)}
          </option>
        </select>
        {localError && <p className="text-xs text-red-300">{localError}</p>}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center gap-2 text-center">
      <span className="text-xs font-medium text-text-secondary">
        {t("predictions.extraTime")}
      </span>
      <input
        id={`et-${match.id}`}
        type="checkbox"
        checked={draft.winMethod === "extra_time"}
        onChange={(e) =>
          onDraftChange({
            ...draft,
            winMethod: e.target.checked ? "extra_time" : null,
            penaltyWinner: null,
          })
        }
        className="h-4 w-4 rounded accent-gold-400"
      />
    </div>
  );
}

function normalizeGroup(group: string | null | undefined): GroupLabel | null {
  const value = group
    ?.trim()
    .toUpperCase()
    .replace(/^GROUP\s+/, "")
    .replace(/^GRUPO\s+/, "");
  return GROUPS.includes(value as GroupLabel) ? (value as GroupLabel) : null;
}

// ── Calendar helpers ───────────────────────────────────────────────────────────

function fmtDate(y: number, m: number, d: number): string {
  return `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
}

function fmtTriggerDate(dateStr: string, locale: Locale): string {
  const [y, m, d] = dateStr.split("-");
  if (locale === "es") return `${d}/${m}/${y}`; // dd/mm/yyyy
  return `${m}/${d}/${y}`; // MM/DD/YYYY (US)
}

function getDayClass(
  isPast: boolean,
  isSelected: boolean,
  isInRange: boolean,
  isToday: boolean,
): string {
  if (isPast) return "cursor-not-allowed text-text-muted opacity-30";
  if (isSelected) return "bg-gold-400 font-bold text-blue-950";
  if (isInRange) return "bg-gold-400/25 text-gold-200";
  if (isToday) return "ring-1 ring-gold-400 text-white hover:bg-white/10";
  return "text-text-secondary hover:bg-white/10 hover:text-white";
}

// ── Match Calendar ─────────────────────────────────────────────────────────────

interface MatchCalendarProps {
  readonly todayStr: string;
  readonly selectedStart: string;
  readonly selectedEnd: string;
  readonly matchDates: Set<string>;
  readonly locale: Locale;
  readonly t: (key: string) => string;
  readonly onRangeChange: (start: string, end: string) => void;
}

function MatchCalendar({
  todayStr,
  selectedStart,
  selectedEnd,
  matchDates,
  locale,
  t,
  onRangeChange,
}: MatchCalendarProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [isOpen, setIsOpen] = useState(false);

  const todayMonth = todayStr.slice(0, 7);
  const [viewMonth, setViewMonth] = useState<string>(todayMonth);
  const [pendingStart, setPendingStart] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    function onMouseDown(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
      }
    }
    document.addEventListener("mousedown", onMouseDown);
    return () => document.removeEventListener("mousedown", onMouseDown);
  }, [isOpen]);

  const [yearStr, monthStr] = viewMonth.split("-");
  const year = Number(yearStr);
  const month = Number(monthStr);

  const firstDayOfMonth = new Date(year, month - 1, 1);
  const daysInMonth = new Date(year, month, 0).getDate();
  const startOffset = (firstDayOfMonth.getDay() + 6) % 7;

  function handleDayClick(dateStr: string) {
    if (dateStr < todayStr) return;
    if (pendingStart !== null && dateStr >= pendingStart) {
      onRangeChange(pendingStart, dateStr);
      setPendingStart(null);
    } else {
      // No pending start, or clicked before it → start a new single-day selection
      setPendingStart(dateStr);
      onRangeChange(dateStr, dateStr);
    }
  }

  function shiftMonth(delta: number) {
    const m2 = month + delta;
    if (m2 < 1) setViewMonth(`${year - 1}-12`);
    else if (m2 > 12) setViewMonth(`${year + 1}-01`);
    else setViewMonth(`${year}-${String(m2).padStart(2, "0")}`);
  }

  const canGoPrev = viewMonth > todayMonth;

  // Format month name and year separately to avoid locale connectors (e.g. "de")
  const monthName = new Intl.DateTimeFormat(
    locale === "es" ? "es-GT" : "en-GB",
    {
      month: "long",
    },
  ).format(firstDayOfMonth);
  const monthLabel = `${monthName} ${year}`;

  const dayHeaders = t("predictions.calendarDayHdrs").split(",");

  type CalendarCell =
    { blank: true; key: string } | { blank: false; dateStr: string };
  const cells: CalendarCell[] = [];
  for (let i = 0; i < startOffset; i++)
    cells.push({ blank: true, key: `${viewMonth}-f${i}` });
  for (let d = 1; d <= daysInMonth; d++)
    cells.push({ blank: false, dateStr: fmtDate(year, month, d) });

  const isRangeMode = selectedStart !== selectedEnd;
  const isSelectionToday =
    selectedStart === todayStr && selectedEnd === todayStr;

  const triggerLabel =
    selectedStart === selectedEnd
      ? fmtTriggerDate(selectedStart, locale)
      : `${fmtTriggerDate(selectedStart, locale)} – ${fmtTriggerDate(selectedEnd, locale)}`;

  return (
    <div ref={containerRef} className="mb-5 flex justify-center">
      <div className="relative">
        {/* Toggle button — date-input style */}
        <button
          type="button"
          onClick={() => setIsOpen((v) => !v)}
          aria-label={t("predictions.calendarToggle")}
          aria-expanded={isOpen}
          aria-haspopup="true"
          className={cn(
            "flex min-w-[220px] items-center gap-3 rounded-lg border px-4 py-2.5 transition-colors",
            isOpen
              ? "border-gold-400/60 bg-white/[0.10] text-white"
              : "border-white/30 bg-white/[0.06] text-text-secondary hover:border-white/50 hover:bg-white/[0.09] hover:text-white",
          )}
        >
          <Calendar className="h-4 w-4 shrink-0 text-gold-300" />
          <span className="flex-1 text-center text-sm font-medium tabular-nums">
            {triggerLabel}
          </span>
          <ChevronDown
            className={cn(
              "h-4 w-4 shrink-0 text-text-muted transition-transform duration-200",
              isOpen && "rotate-180",
            )}
          />
        </button>

        {/* Floating dropdown — centred under the trigger */}
        {isOpen && (
          <div className="absolute left-1/2 top-full z-50 mt-1 w-72 -translate-x-1/2 rounded-2xl border border-white/10 bg-[#07111F] p-3 shadow-xl shadow-black/40">
            {/* Month navigation */}
            <div className="mb-3 flex items-center justify-between">
              <button
                type="button"
                onClick={() => shiftMonth(-1)}
                disabled={!canGoPrev}
                aria-label={t("predictions.calendarPrevMonth")}
                className={cn(
                  "rounded p-1.5 transition-colors",
                  canGoPrev
                    ? "text-text-secondary hover:bg-white/10 hover:text-white"
                    : "cursor-not-allowed text-text-muted opacity-30",
                )}
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
              <span className="text-sm font-semibold capitalize text-white">
                {monthLabel}
              </span>
              <button
                type="button"
                onClick={() => shiftMonth(1)}
                aria-label={t("predictions.calendarNextMonth")}
                className="rounded p-1.5 text-text-secondary transition-colors hover:bg-white/10 hover:text-white"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>

            {/* Day-of-week headers */}
            <div className="mb-1 grid grid-cols-7 text-center">
              {dayHeaders.map((d) => (
                <span
                  key={d}
                  className="py-1 text-[10px] font-medium uppercase tracking-wide text-text-muted"
                >
                  {d}
                </span>
              ))}
            </div>

            {/* Day grid */}
            <div className="grid grid-cols-7 gap-y-0.5">
              {cells.map((cell) => {
                if (cell.blank) return <span key={cell.key} />;
                const { dateStr } = cell;
                const isPast = dateStr < todayStr;
                const isToday = dateStr === todayStr;
                const isSelected =
                  dateStr === selectedStart || dateStr === selectedEnd;
                const isInRange =
                  isRangeMode &&
                  dateStr > selectedStart &&
                  dateStr < selectedEnd;
                const hasMatch = matchDates.has(dateStr);
                const day = Number(dateStr.slice(-2));
                return (
                  <div
                    key={dateStr}
                    className="relative flex flex-col items-center"
                  >
                    <button
                      type="button"
                      onClick={() => handleDayClick(dateStr)}
                      disabled={isPast}
                      aria-label={dateStr}
                      aria-pressed={isSelected || isInRange}
                      className={cn(
                        "h-8 w-8 rounded text-xs font-medium transition-colors",
                        getDayClass(isPast, isSelected, isInRange, isToday),
                      )}
                    >
                      {day}
                    </button>
                    {hasMatch && (
                      <span
                        className={cn(
                          "mt-0.5 h-1 w-1 rounded-full",
                          isSelected ? "bg-blue-950" : "bg-gold-400/70",
                        )}
                      />
                    )}
                  </div>
                );
              })}
            </div>

            {/* Footer: "Partidos de hoy" first, range hint below */}
            <div className="mt-3 flex flex-col items-center gap-1">
              {!isSelectionToday && (
                <button
                  type="button"
                  onClick={() => {
                    setPendingStart(null);
                    setViewMonth(todayMonth);
                    onRangeChange(todayStr, todayStr);
                  }}
                  className="text-[10px] text-text-muted underline underline-offset-2 hover:text-text-secondary"
                >
                  {t("predictions.calendarGoToday")}
                </button>
              )}
              {pendingStart && (
                <p className="text-[10px] text-gold-300">
                  {t("predictions.calendarRangeHint")}
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
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
