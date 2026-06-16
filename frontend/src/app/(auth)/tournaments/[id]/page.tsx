"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Check,
  Copy,
  Crown,
  Loader2,
  LogOut,
  Medal,
  Settings,
  Trophy,
  UserCheck,
  UserX,
  Users,
  X,
} from "lucide-react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { api } from "@/lib/api";
import type { MemberResponse } from "@/lib/api-types";
import { cn } from "@/lib/utils";
import { useCurrency } from "@/hooks/useCurrency";
import { LoadingState } from "@/components/shared/LoadingState";
import { StatusBadge } from "@/components/shared/StatusBadge";
import { TournamentSettingsModal } from "@/components/groups/TournamentSettingsModal";
import { useI18n } from "@/lib/i18n";
import { LivePredictionsCarousel } from "@/components/groups/LivePredictionsCarousel";
export default function TournamentDetailPage() {
  const { getToken } = useAuth();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { t } = useI18n();
  const { fmt } = useCurrency();
  const groupId = Number(params.id);

  const [copied, setCopied] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [leaveError, setLeaveError] = useState<string | null>(null);
  const [approvingMember, setApprovingMember] = useState<MemberResponse | null>(
    null,
  );
  const [showSettings, setShowSettings] = useState(false);

  const groupQuery = useQuery({
    queryKey: ["group", groupId],
    queryFn: async () => {
      const token = await getToken();
      return api.getGroup(token!, groupId);
    },
    enabled: !Number.isNaN(groupId),
  });

  const leaderboardQuery = useQuery({
    queryKey: ["group-leaderboard", groupId],
    queryFn: async () => {
      const token = await getToken();
      return api.getGroupLeaderboard(token!, groupId, true);
    },
    enabled: !Number.isNaN(groupId),
  });

  const membersQuery = useQuery({
    queryKey: ["group-members", groupId],
    queryFn: async () => {
      const token = await getToken();
      return api.getGroupMembers(token!, groupId);
    },
    enabled: !Number.isNaN(groupId),
  });

  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: async () => {
      const token = await getToken();
      return api.getMe(token!);
    },
  });

  const leaveMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      return api.leaveGroup(token!, groupId);
    },
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ["my-groups"] });
      queryClient.removeQueries({ queryKey: ["group", groupId] });
      router.replace("/dashboard");
    },
    onError: (err: unknown) => {
      const msg =
        (err as { message?: string })?.message ?? "Error al salir del grupo";
      setLeaveError(msg);
    },
  });

  const approveMutation = useMutation({
    mutationFn: async (membershipId: number) => {
      const token = await getToken();
      return api.approveGroupMember(token!, groupId, membershipId);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["group-members", groupId] });
      setApprovingMember(null);
    },
  });

  const rejectMutation = useMutation({
    mutationFn: async (membershipId: number) => {
      const token = await getToken();
      return api.rejectGroupMember(token!, groupId, membershipId);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["group-members", groupId] });
      setApprovingMember(null);
    },
  });

  async function copyCode(code: string) {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  const group = groupQuery.data;
  const entries = leaderboardQuery.data?.entries ?? [];
  const activePaid = leaderboardQuery.data?.active_paid_members ?? 0;
  const members = membersQuery.data ?? [];
  const activeMembers = members.filter((m) => m.status === "active");
  const pendingMembers = members.filter((m) => m.status === "pending");
  const isLoading = groupQuery.isLoading;

  const myUserId = meQuery.data?.id;
  const isOwner = group && myUserId != null && myUserId === group.owner_user_id;

  const statusKey = group?.is_premium ? "premium" : "free";

  if (!Number.isNaN(groupId) && groupQuery.isError) {
    return (
      <div className="py-20 text-center">
        <p className="text-text-muted">{t("common.notFound")}</p>
        <Link href="/dashboard" className="btn-gold mt-4 px-4 py-2 text-sm">
          {t("common.back")}
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link
          href="/dashboard"
          className="rounded p-1 text-text-muted transition-colors hover:text-white"
        >
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <div className="min-w-0 flex-1">
          {isLoading ? (
            <div className="h-6 w-48 animate-pulse rounded bg-white/10" />
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-xl font-semibold text-white">
                {group?.name}
              </h1>
              {group && <StatusBadge status={statusKey} size="sm" />}
            </div>
          )}
        </div>
        {/* Gear icon — owner only */}
        {isOwner && group && (
          <button
            type="button"
            onClick={() => setShowSettings(true)}
            className="rounded-lg border border-white/10 p-2 text-text-muted transition-colors hover:border-white/20 hover:text-white"
            aria-label={t("group.settings")}
          >
            <Settings className="h-4 w-4" />
          </button>
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {/* Invite code card */}
        {group?.invite_code && (
          <div className="card p-4">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
              {t("groups.inviteCodeLabel")}
            </p>
            <div className="flex items-center gap-2">
              <span className="flex-1 font-mono text-lg font-bold tracking-widest text-gold-300">
                {group.invite_code}
              </span>
              <button
                type="button"
                onClick={() => copyCode(group.invite_code)}
                className="rounded p-1 text-text-muted transition-colors hover:text-white"
              >
                {copied ? (
                  <Check className="h-4 w-4 text-green-400" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </button>
            </div>
            <p className="mt-1 text-[11px] text-text-muted">
              {t("groups.inviteCodeHint")}
            </p>
          </div>
        )}

        {/* Members count */}
        <div className="card flex items-center gap-3 p-4">
          <Users className="h-8 w-8 shrink-0 text-blue-300" />
          <div className="min-w-0 flex-1">
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold text-white">
                {activeMembers.length}
              </p>
              {pendingMembers.length > 0 && (
                <span className="text-sm font-medium text-yellow-400">
                  +{pendingMembers.length}
                </span>
              )}
            </div>
            <p className="text-xs text-text-muted">
              {t("group.membersActive")}
            </p>
            {pendingMembers.length > 0 && (
              <p className="text-[10px] text-yellow-400/70">
                {t("group.membersPending")}
              </p>
            )}
          </div>
        </div>

        {/* Free mode indicator */}
        {!group?.is_premium && (
          <div className="card flex items-center gap-3 p-4">
            <Trophy className="h-8 w-8 shrink-0 text-text-muted" />
            <div>
              <p className="text-lg font-semibold text-text-muted">
                {t("status.free")}
              </p>
            </div>
          </div>
        )}

        {/* Prize pool — general mode */}
        {group?.mode_general && (
          <div className="card flex items-center gap-3 p-4">
            <Trophy className="h-8 w-8 shrink-0 text-gold-400" />
            <div>
              <p
                className="text-2xl font-bold text-white"
                suppressHydrationWarning
              >
                {group ? fmt(group.entry_fee * activePaid) : "—"}
              </p>
              <p className="text-xs text-text-muted">
                {t("group.prizePoolGeneral")}
              </p>
            </div>
          </div>
        )}
      </div>

      {/* Live predictions carousel — only renders when ≥1 match is in_progress */}
      {!Number.isNaN(groupId) && <LivePredictionsCarousel groupId={groupId} />}

      <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
        {/* Leaderboard */}
        <section className="panel p-4 sm:p-5">
          <div className="mb-4 flex items-center gap-2">
            <Medal className="h-5 w-5 text-gold-300" />
            <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
              {t("group.leaderboard")}
            </h2>
          </div>

          {leaderboardQuery.isLoading && <LoadingState rows={5} />}

          {!leaderboardQuery.isLoading && entries.length === 0 && (
            <p className="py-8 text-center text-sm text-text-muted">
              {t("group.noScores")}
            </p>
          )}

          {entries.length > 0 && <LeaderboardTable entries={entries} t={t} />}
        </section>

        {/* Members list */}
        <section className="panel p-4 sm:p-5">
          <div className="mb-4 flex items-center gap-2">
            <Users className="h-5 w-5 text-blue-300" />
            <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
              {t("group.members")}
            </h2>
          </div>

          {membersQuery.isLoading && <LoadingState rows={3} />}

          {!membersQuery.isLoading &&
            activeMembers.length === 0 &&
            pendingMembers.length === 0 && (
              <p className="py-4 text-center text-sm text-text-muted">
                {t("group.noMembers")}
              </p>
            )}

          {/* Active members */}
          {activeMembers.length > 0 && (
            <div className="space-y-1">
              {activeMembers.map((member) => (
                <div
                  key={member.id}
                  className="flex items-center gap-2 rounded-lg px-2 py-2"
                >
                  {member.role === "owner" && (
                    <Crown className="h-3.5 w-3.5 shrink-0 text-gold-400" />
                  )}
                  <span
                    className={cn(
                      "flex-1 truncate text-sm",
                      member.role === "owner"
                        ? "text-gold-200"
                        : "text-text-primary",
                    )}
                  >
                    {member.display_name}
                  </span>
                </div>
              ))}
            </div>
          )}

          {/* Pending members */}
          {pendingMembers.length > 0 && (
            <div className="mt-3">
              <p className="mb-1.5 px-2 text-[10px] font-semibold uppercase tracking-wide text-yellow-400/70">
                {t("group.membersPending")}
              </p>
              <div className="space-y-1">
                {pendingMembers.map((member) => (
                  <div
                    key={member.id}
                    className="flex items-center gap-2 rounded-lg px-2 py-2"
                  >
                    <span className="flex-1 truncate text-sm text-text-secondary">
                      {member.display_name}
                    </span>
                    <button
                      type="button"
                      onClick={() => setApprovingMember(member)}
                      className="rounded-full border border-yellow-400/30 bg-yellow-400/10 px-2 py-0.5 text-[9px] font-semibold uppercase text-yellow-300 transition-colors hover:border-yellow-400/60 hover:bg-yellow-400/20"
                    >
                      {t("groups.pendingBadge")}
                    </button>
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="mt-6 border-t border-white/10 pt-4">
            {confirmLeave ? (
              <div className="rounded-xl border border-red-400/20 bg-red-400/5 p-4 space-y-3">
                <div className="flex items-start gap-2">
                  <LogOut className="mt-0.5 h-4 w-4 shrink-0 text-red-400" />
                  <div>
                    <p className="text-sm font-medium text-red-300">
                      {t("group.leaveConfirm")}
                    </p>
                    <p className="mt-0.5 text-[11px] text-text-muted">
                      {t("group.leaveConfirmDetail")}
                    </p>
                  </div>
                </div>
                {leaveError && (
                  <p className="rounded-lg bg-red-500/10 px-3 py-2 text-[11px] text-red-400 border border-red-400/20">
                    {leaveError}
                  </p>
                )}
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setConfirmLeave(false);
                      setLeaveError(null);
                    }}
                    className="flex-1 rounded-lg border border-white/10 px-3 py-2 text-xs text-text-muted hover:text-white"
                  >
                    {t("common.cancel")}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setLeaveError(null);
                      leaveMutation.mutate();
                    }}
                    disabled={leaveMutation.isPending}
                    className="flex-1 rounded-lg bg-red-500/80 px-3 py-2 text-xs font-medium text-white hover:bg-red-500 disabled:opacity-50"
                  >
                    {leaveMutation.isPending
                      ? t("common.saving")
                      : t("group.leaveConfirmBtn")}
                  </button>
                </div>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => setConfirmLeave(true)}
                className="flex w-full items-center justify-center gap-2 rounded-lg border border-red-400/20 px-3 py-2 text-xs text-red-400 transition-colors hover:border-red-400/40 hover:bg-red-400/10"
              >
                <LogOut className="h-3.5 w-3.5" />
                {t("group.leave")}
              </button>
            )}
          </div>
        </section>
      </div>

      {/* Single-member approval popup */}
      {approvingMember && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <button
            type="button"
            aria-label="Close"
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setApprovingMember(null)}
          />
          <div className="relative w-full max-w-sm rounded-2xl border border-white/10 bg-[#0b1929] p-6 shadow-2xl">
            <div className="mb-5 relative text-center">
              <button
                type="button"
                onClick={() => setApprovingMember(null)}
                className="absolute right-0 top-0 rounded p-1 text-text-muted transition-colors hover:text-white"
              >
                <X className="h-5 w-5" />
              </button>
              <p className="text-xs font-semibold uppercase tracking-wide text-gold-300">
                {t("groups.pendingTitle")}
              </p>
              <h2 className="mt-0.5 text-lg font-semibold text-white">
                {approvingMember.display_name}
              </h2>
            </div>

            <div className="flex gap-3">
              <button
                type="button"
                disabled={approveMutation.isPending || rejectMutation.isPending}
                onClick={() => approveMutation.mutate(approvingMember.id)}
                className="flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-green-500/80 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-green-500 disabled:opacity-50"
              >
                {approveMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <UserCheck className="h-4 w-4" />
                )}
                {t("groups.approve")}
              </button>
              <button
                type="button"
                disabled={approveMutation.isPending || rejectMutation.isPending}
                onClick={() => rejectMutation.mutate(approvingMember.id)}
                className="flex flex-1 items-center justify-center gap-1.5 rounded-lg border border-red-400/30 px-4 py-2.5 text-sm font-medium text-red-400 transition-colors hover:border-red-400/60 hover:bg-red-400/10 disabled:opacity-50"
              >
                {rejectMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <UserX className="h-4 w-4" />
                )}
                {t("groups.reject")}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Tournament settings modal — owner only */}
      {showSettings && group && (
        <TournamentSettingsModal
          group={group}
          memberCount={activeMembers.length}
          onClose={() => setShowSettings(false)}
        />
      )}
    </div>
  );
}

// Round metadata: key → short label (es) and label key for i18n
const ROUND_DEFS: { key: string; i18nKey: string }[] = [
  { key: "1", i18nKey: "group.round1" },
  { key: "2", i18nKey: "group.round2" },
  { key: "3", i18nKey: "group.round3" },
  { key: "4", i18nKey: "group.round4" },
  { key: "5", i18nKey: "group.round5" },
  { key: "6", i18nKey: "group.round6" },
  { key: "7", i18nKey: "group.round7" },
  { key: "8", i18nKey: "group.round8" },
  { key: "9", i18nKey: "group.round9" },
];

interface LeaderboardTableProps {
  readonly entries: import("@/lib/api-types").LeaderboardEntry[];
  readonly t: ReturnType<typeof useI18n>["t"];
}

function LeaderboardTable({ entries, t }: LeaderboardTableProps) {
  // Only render columns for rounds that have at least one non-zero score
  const activeRounds = ROUND_DEFS.filter((rd) =>
    entries.some((e) => (e.round_points?.[rd.key] ?? 0) > 0),
  );

  return (
    <div className="overflow-x-auto -mx-4 sm:-mx-5">
      <table className="w-full min-w-max border-collapse text-sm">
        <thead>
          <tr className="border-b border-white/10">
            <th className="px-4 py-2 text-left text-[10px] font-semibold uppercase tracking-wide text-text-muted sm:px-5">
              #
            </th>
            <th className="px-2 py-2 text-left text-[10px] font-semibold uppercase tracking-wide text-text-muted">
              {t("group.members")}
            </th>
            {activeRounds.map((rd) => (
              <th
                key={rd.key}
                className="px-2 py-2 text-center text-[10px] font-semibold uppercase tracking-wide text-text-muted"
              >
                {t(rd.i18nKey)}
              </th>
            ))}
            <th className="px-4 py-2 text-center text-[10px] font-semibold uppercase tracking-wide text-gold-400 sm:px-5">
              Total
            </th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => (
            <tr
              key={entry.user_id}
              className={cn(
                "border-b border-white/5 transition-colors",
                entry.prize_winner ? "bg-gold-400/10" : "hover:bg-white/[0.03]",
              )}
            >
              {/* Rank */}
              <td className="px-4 py-2.5 sm:px-5">
                <RankBadge rank={entry.rank} />
              </td>
              {/* Name */}
              <td className="max-w-[140px] px-2 py-2.5">
                <span
                  className={cn(
                    "block truncate",
                    entry.prize_winner
                      ? "font-semibold text-gold-200"
                      : "text-text-primary",
                  )}
                >
                  {entry.user_name}
                </span>
              </td>
              {/* Per-round points */}
              {activeRounds.map((rd) => {
                const pts = entry.round_points?.[rd.key] ?? 0;
                return (
                  <td key={rd.key} className="px-2 py-2.5 text-center">
                    <span
                      className={cn(
                        "text-xs tabular-nums",
                        pts > 0 ? "text-white" : "text-text-muted/40",
                      )}
                    >
                      {pts > 0 ? pts : "—"}
                    </span>
                  </td>
                );
              })}
              {/* Total */}
              <td className="px-4 py-2.5 text-center sm:px-5">
                <span className="font-score text-sm font-semibold text-white tabular-nums">
                  {entry.total_points}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

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
