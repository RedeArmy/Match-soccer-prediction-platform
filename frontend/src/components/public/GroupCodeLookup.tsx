"use client";

import { useState } from "react";
import { ArrowLeft, Medal, Search } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { LoadingState } from "@/components/shared/LoadingState";
import type { LeaderboardEntry } from "@/lib/api-types";

// ── Types ─────────────────────────────────────────────────────────────────────

interface PublicLeaderboardResponse {
  group_name: string;
  entries: LeaderboardEntry[];
}

// ── Sub-components ────────────────────────────────────────────────────────────

function RankBadge({ rank }: Readonly<{ rank: number }>) {
  if (rank === 1) return <span className="text-base">🥇</span>;
  if (rank === 2) return <span className="text-base">🥈</span>;
  if (rank === 3) return <span className="text-base">🥉</span>;
  return (
    <span className="w-5 text-center text-xs font-bold text-text-muted">
      {rank}
    </span>
  );
}

interface TableProps {
  readonly groupName: string;
  readonly entries: LeaderboardEntry[];
  readonly onClose: () => void;
}

function LeaderboardTable({ groupName, entries, onClose }: TableProps) {
  const { t } = useI18n();

  return (
    <div className="panel overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 border-b border-white/10 p-4 sm:p-5">
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-text-muted transition-colors hover:text-white"
          aria-label={t("tournaments.lookupClose")}
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <Medal className="h-5 w-5 shrink-0 text-gold-300" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold text-white">
            {groupName}
          </p>
          <p className="text-[10px] text-text-muted">
            {t("tournaments.lookupCurrentLabel")}
          </p>
        </div>
      </div>

      {entries.length === 0 ? (
        <p className="py-10 text-center text-sm text-text-muted">
          {t("tournaments.lookupEmpty")}
        </p>
      ) : (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-white/10">
              <th className="px-4 py-2 text-left text-[10px] font-semibold uppercase tracking-wide text-text-muted sm:px-5">
                #
              </th>
              <th className="px-2 py-2 text-left text-[10px] font-semibold uppercase tracking-wide text-text-muted">
                {t("tournaments.lookupColPlayer")}
              </th>
              <th className="px-4 py-2 text-center text-[10px] font-semibold uppercase tracking-wide text-gold-400 sm:px-5">
                Total
              </th>
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr
                key={entry.user_id}
                className="border-b border-white/5 transition-colors hover:bg-white/[0.03]"
              >
                <td className="px-4 py-2.5 sm:px-5">
                  <RankBadge rank={entry.rank} />
                </td>
                <td className="px-2 py-2.5">
                  <span className="block truncate text-text-primary">
                    {entry.user_name}
                  </span>
                </td>
                <td className="px-4 py-2.5 text-center sm:px-5">
                  <span className="font-score text-sm font-semibold text-white tabular-nums">
                    {entry.total_points}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

export function GroupCodeLookup() {
  const { t } = useI18n();

  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<PublicLeaderboardResponse | null>(null);

  async function handleLookup() {
    const trimmed = code.trim().toUpperCase();
    if (!trimmed) return;

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const res = await fetch(
        `/api/public/groups/leaderboard?code=${encodeURIComponent(trimmed)}`,
        { cache: "no-store" },
      );

      if (res.status === 404) {
        setError(t("tournaments.lookupErrorNotFound"));
        return;
      }
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as {
          error?: { message?: string };
        };
        setError(body.error?.message ?? t("tournaments.lookupErrorGeneric"));
        return;
      }

      const data: PublicLeaderboardResponse = await res.json();
      setResult(data);
    } catch {
      setError(t("tournaments.lookupErrorNetwork"));
    } finally {
      setLoading(false);
    }
  }

  function handleReset() {
    setResult(null);
    setError(null);
    setCode("");
  }

  if (result) {
    return (
      <LeaderboardTable
        groupName={result.group_name}
        entries={result.entries}
        onClose={handleReset}
      />
    );
  }

  return (
    <div className="panel p-5">
      <div className="mb-4 flex items-center gap-2">
        <Search className="h-5 w-5 text-gold-300" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
          {t("tournaments.lookupTitle")}
        </h2>
      </div>

      <p className="mb-4 text-xs text-text-muted">
        {t("tournaments.lookupDesc")}
      </p>

      <div className="flex gap-2">
        <input
          type="text"
          value={code}
          onChange={(e) => {
            setCode(e.target.value.toUpperCase());
            setError(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleLookup();
          }}
          placeholder={t("tournaments.lookupPlaceholder")}
          maxLength={12}
          className="input-base flex-1 font-mono tracking-widest text-gold-200 placeholder:text-text-muted placeholder:tracking-normal placeholder:font-sans"
          aria-label={t("tournaments.lookupPlaceholder")}
        />
        <button
          type="button"
          onClick={handleLookup}
          disabled={loading || !code.trim()}
          className="btn-gold px-4 py-2 text-sm disabled:opacity-50"
        >
          {loading ? (
            <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-bg-base border-t-transparent" />
          ) : (
            t("tournaments.lookupBtn")
          )}
        </button>
      </div>

      {error && (
        <p className="mt-3 rounded-lg border border-red-400/20 bg-red-400/5 px-3 py-2 text-xs text-red-400">
          {error}
        </p>
      )}

      {loading && (
        <div className="mt-4">
          <LoadingState rows={4} />
        </div>
      )}
    </div>
  );
}
