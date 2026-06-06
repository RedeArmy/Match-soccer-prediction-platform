"use client";

import { useEffect, useMemo, useState } from "react";
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
type Filter = "all" | "pending" | "saved";
type GroupLabel = "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J" | "K" | "L";

const GROUPS: GroupLabel[] = ["A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"];

export function PredictionPanel() {
  const { getToken } = useAuth();
  const queryClient = useQueryClient();
  const { t, teamName, phaseName, formatKickoff } = useI18n();
  const [filter, setFilter] = useState<Filter>("all");
  const [selectedGroup, setSelectedGroup] = useState<GroupLabel>("A");
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
  });

  const predictionsQuery = useQuery({
    queryKey: ["my-predictions"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMyPredictions(token!);
    },
  });

  const predictionByMatch = useMemo(() => {
    const map = new Map<number, PredictionResponse>();
    for (const prediction of predictionsQuery.data ?? []) {
      map.set(prediction.match_id, prediction);
    }
    return map;
  }, [predictionsQuery.data]);

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
    const ts = (s: string | null) => (s ? new Date(s).getTime() : Infinity)
    return [...(matchesQuery.data ?? [])]
      .filter((m) => isPhaseVisible(m.phase))
      .sort((a, b) => ts(a.kickoff_at) - ts(b.kickoff_at));
  }, [matchesQuery.data]);

  const groupFilteredMatches = sortedMatches.filter(
    (match) => normalizeGroup(match.group_label) === selectedGroup,
  );

  const visibleMatches = groupFilteredMatches.filter((match) => {
    const hasPrediction = predictionByMatch.has(match.id);
    if (filter === "pending") return !hasPrediction;
    if (filter === "saved") return hasPrediction;
    return true;
  });

const isLoading = matchesQuery.isLoading || predictionsQuery.isLoading;

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

      <div className="mb-5 rounded-2xl border border-white/10 bg-[#07111F] p-3 sm:p-4">
        <div className="mb-3 flex items-end justify-between gap-2">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-gold-300">
              {t("predictions.groupSelector")}
            </p>
            <p className="text-sm text-text-secondary">
              {`${t("predictions.groupSelected")} ${selectedGroup}`}
            </p>
          </div>
          <span className="text-xs text-text-muted">
            {visibleMatches.length} {t("predictions.matches")}
          </span>
        </div>

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

      {isLoading && <LoadingState rows={4} />}

      {!isLoading && visibleMatches.length === 0 && (
        <EmptyState
          title={t("predictions.noMatches")}
          description={t("predictions.noMatchesDesc")}
          icon={<Target className="h-10 w-10" />}
        />
      )}

      {!isLoading && visibleMatches.length > 0 && (
        <div className="grid gap-3">
          {visibleMatches.map((match) => {
            const prediction = predictionByMatch.get(match.id);
            const draft = drafts[match.id] ?? {
              home: prediction?.home_score ?? 0,
              away: prediction?.away_score ?? 0,
            };
            const locked =
              (match.kickoff_at !== null && new Date(match.kickoff_at).getTime() <= Date.now()) ||
              match.status !== "scheduled";
            const pending =
              mutation.isPending && mutation.variables?.match.id === match.id;

            return (
              <article
                key={match.id}
                className="rounded border border-white/10 bg-white/[0.025] p-4"
              >
                <div className="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-center">
                  <div className="min-w-0">
                    <div className="mb-3 flex flex-wrap items-center gap-2">
                      <StatusBadge status={match.status} size="sm" />
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
                        {prediction
                          ? t("predictions.saved")
                          : t("predictions.unsaved")}
                      </span>
                      {locked && (
                        <span className="text-[10px] uppercase text-red-300">
                          {t("predictions.locked")}
                        </span>
                      )}
                    </div>

                    <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-3">
                      <TeamLabel label={teamName(match.home_team)} align="right" />
                      <span className="font-score text-xs text-text-muted">
                        vs
                      </span>
                      <TeamLabel label={teamName(match.away_team)} align="left" />
                    </div>

                    <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-muted">
                      <span className="inline-flex items-center gap-1.5">
                        <CalendarClock className="h-3.5 w-3.5" />
                        {t("predictions.kickoff")}:{" "}
                        <span suppressHydrationWarning>{formatKickoff(match.kickoff_at)}</span>
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
                      <MatchCountdown kickoffAt={match.kickoff_at} />
                      {prediction?.points !== null &&
                        prediction?.points !== undefined && (
                          <span className="text-gold-300">
                            {prediction.points} {t("predictions.points")}
                          </span>
                        )}
                    </div>
                  </div>

                  <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                    <ScoreInput
                      label={t("predictions.home")}
                      value={draft.home}
                      disabled={locked}
                      onChange={(value) =>
                        setDrafts((current) => ({
                          ...current,
                          [match.id]: { ...draft, home: value },
                        }))
                      }
                    />
                    <ScoreInput
                      label={t("predictions.away")}
                      value={draft.away}
                      disabled={locked}
                      onChange={(value) =>
                        setDrafts((current) => ({
                          ...current,
                          [match.id]: { ...draft, away: value },
                        }))
                      }
                    />
                    <button
                      type="button"
                      disabled={locked || pending}
                      onClick={() => mutation.mutate({ match, draft })}
                      className="btn-gold min-w-36 px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <Save className="h-4 w-4" />
                      {(() => {
                        if (pending) return t("common.saving");
                        if (prediction) return t("predictions.update");
                        return t("predictions.submit");
                      })()}
                    </button>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      )}

      <p className="mt-4 text-xs text-text-muted">
        {t("predictions.exactHint")}
      </p>
      </div>
    </section>
  );
}

function normalizeGroup(group: string | null | undefined): GroupLabel | null {
  const value = group?.trim().toUpperCase().replace(/^GROUP\s+/, "").replace(/^GRUPO\s+/, "");
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

function MatchCountdown({ kickoffAt }: Readonly<{ kickoffAt: string | null | undefined }>) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  if (!kickoffAt) return null;
  const diff = new Date(kickoffAt).getTime() - now;
  if (diff <= 0) return null;

  const days  = Math.floor(diff / 86_400_000);
  const hours = Math.floor((diff % 86_400_000) / 3_600_000);
  const mins  = Math.floor((diff % 3_600_000) / 60_000);
  const secs  = Math.floor((diff % 60_000) / 1_000);

  const label =
    days > 0  ? `${days}d ${hours}h ${mins}m` :
    hours > 0 ? `${hours}h ${mins}m ${secs}s` :
                `${mins}m ${secs}s`;

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
        max={30}
        inputMode="numeric"
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(Math.max(0, Number(event.target.value)))}
        className="input-base h-10 text-center font-score text-lg disabled:cursor-not-allowed disabled:opacity-55"
      />
    </label>
  );
}
