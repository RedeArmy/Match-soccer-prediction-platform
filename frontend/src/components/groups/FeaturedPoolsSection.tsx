"use client";

import React, { useCallback, useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useQuery } from "@tanstack/react-query";
import { Star, Trophy, Users } from "lucide-react";
import Link from "next/link";
import { api } from "@/lib/api";
import type { GroupResponse } from "@/lib/api-types";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { LoadingState } from "@/components/shared/LoadingState";
import { HorizontalCarousel } from "@/components/shared/HorizontalCarousel";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useCurrency } from "@/hooks/useCurrency";

const LS_KEY = "wc26_featured_group_ids";

function useFeatured() {
  const [ids, setIds] = useState<Set<number>>(() => {
    if (globalThis.window === undefined) return new Set();
    try {
      const raw = localStorage.getItem(LS_KEY);
      return raw ? new Set<number>(JSON.parse(raw)) : new Set();
    } catch {
      return new Set();
    }
  });

  const toggle = useCallback((id: number) => {
    setIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      try {
        localStorage.setItem(LS_KEY, JSON.stringify([...next]));
      } catch {
        /* noop */
      }
      return next;
    });
  }, []);

  return { ids, toggle };
}

export function FeaturedPoolsSection() {
  const { getToken } = useAuth();
  const { t } = useI18n();
  const { fmt } = useCurrency();
  const { ids: featuredIds, toggle } = useFeatured();

  const groupsQuery = useQuery({
    queryKey: ["my-groups"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMyGroups(token!);
    },
  });

  const groups = groupsQuery.data ?? [];
  const featured = groups.filter((g) => featuredIds.has(g.id));
  const mine = groups.filter((g) => g.membership_status === "active");

  return (
    <div className="space-y-8">
      {/* ── Featured carousel — only shown when at least one is starred ── */}
      {featured.length > 0 && (
        <section>
          <div className="mb-3 flex items-center gap-2">
            <Star className="h-4 w-4 fill-gold-400 text-gold-400" />
            <h2 className="text-sm font-semibold uppercase tracking-wide text-gold-300">
              {t("tournaments.featuredPools")}
            </h2>
          </div>

          <HorizontalCarousel>
            {featured.map((g) => (
              <PoolCard
                key={g.id}
                group={g}
                isFeatured
                onToggleFeatured={() => toggle(g.id)}
                fmt={fmt}
                t={t}
              />
            ))}
          </HorizontalCarousel>
        </section>
      )}

      {/* ── My pools ──────────────────────────────────────────────── */}
      <section>
        <div className="mb-3 flex items-center gap-2">
          <Trophy className="h-4 w-4 text-text-secondary" />
          <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
            {t("tournaments.myPools")}
          </h2>
        </div>

        {groupsQuery.isLoading && <LoadingState rows={3} />}

        {!groupsQuery.isLoading && mine.length === 0 && (
          <p className="rounded-xl border border-white/10 bg-white/[0.02] px-4 py-6 text-center text-sm text-text-muted">
            {t("tournaments.emptyTitle")}
          </p>
        )}

        {mine.length > 0 && (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {mine.map((g) => (
              <PoolCard
                key={g.id}
                group={g}
                isFeatured={featuredIds.has(g.id)}
                onToggleFeatured={() => toggle(g.id)}
                fmt={fmt}
                t={t}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

// ── Pool card ────────────────────────────────────────────────────────────────

interface PoolCardProps {
  readonly group: GroupResponse;
  readonly isFeatured: boolean;
  readonly onToggleFeatured: () => void;
  readonly fmt: (cents: number) => string;
  readonly t: ReturnType<typeof useI18n>["t"];
}

function PoolCard({
  group,
  isFeatured,
  onToggleFeatured,
  fmt,
  t,
}: PoolCardProps) {
  const prizePool =
    group.is_premium && group.mode_general
      ? group.entry_fee * group.active_member_count
      : 0;

  const modes: string[] = [];
  if (group.mode_general) modes.push(t("group.modeGeneral"));
  if (group.mode_round) modes.push(t("group.modeRound"));

  return (
    <article className="card overflow-hidden transition-colors hover:border-gold-400/30">
      <div className="h-2 bg-gradient-to-r from-red-500 via-gold-400 to-green-400" />
      <div className="p-5">
        {/* Top row: [Gratis/Premium] ——————————— [Líder] [★] */}
        <div className="mb-2 flex items-center justify-between gap-2">
          <StatusBadge
            status={group.is_premium ? "premium" : "free"}
            size="sm"
          />
          <div className="flex items-center gap-1">
            {group.role === "owner" && (
              <span className="rounded border border-gold-400/20 bg-gold-400/5 px-1.5 py-0.5 text-[10px] font-medium text-gold-400">
                {t("group.ownerLabel")}
              </span>
            )}
            <button
              type="button"
              onClick={onToggleFeatured}
              title={
                isFeatured
                  ? t("tournaments.removeFeatured")
                  : t("tournaments.addFeatured")
              }
              className={cn(
                "rounded p-1 transition-colors",
                isFeatured
                  ? "text-gold-400 hover:text-gold-300"
                  : "text-text-muted hover:text-gold-400",
              )}
            >
              <Star className={cn("h-4 w-4", isFeatured && "fill-gold-400")} />
            </button>
          </div>
        </div>

        {/* Name */}
        <h2 className="mb-2 text-base font-semibold leading-tight text-white">
          {group.name}
        </h2>

        {/* Mode chips */}
        {modes.length > 0 && (
          <div className="mb-3 flex flex-wrap gap-1">
            {modes.map((m) => (
              <span
                key={m}
                className="rounded border border-white/10 bg-white/[0.04] px-1.5 py-0.5 text-[10px] text-text-muted"
              >
                {m}
              </span>
            ))}
          </div>
        )}

        {/* Stats */}
        <div className="space-y-1.5 text-xs text-text-muted">
          <div className="flex items-center gap-1.5">
            <Users className="h-3.5 w-3.5 text-blue-300" />
            <span>
              <span className="font-medium text-white">
                {group.active_member_count}
              </span>{" "}
              {t("group.membersActive")}
            </span>
          </div>

          {group.is_premium && (
            <div className="flex items-center gap-1.5">
              <Trophy className="h-3.5 w-3.5 text-gold-400" />
              <span>
                {t("tournaments.entry")}:{" "}
                <span
                  className="font-medium text-gold-400"
                  suppressHydrationWarning
                >
                  {fmt(group.entry_fee)}
                </span>
              </span>
            </div>
          )}

          {prizePool > 0 && (
            <div className="flex items-center gap-1.5">
              <Trophy className="h-3.5 w-3.5 text-green-400" />
              <span>
                {t("group.prizePoolGeneral")}:{" "}
                <span
                  className="font-medium text-green-300"
                  suppressHydrationWarning
                >
                  {fmt(prizePool)}
                </span>
              </span>
            </div>
          )}
        </div>

        <Link
          href={`/tournaments/${group.id}`}
          className="btn-gold mt-4 block w-full py-2 text-center text-sm"
        >
          {t("tournaments.viewDetail")}
        </Link>
      </div>
    </article>
  );
}
