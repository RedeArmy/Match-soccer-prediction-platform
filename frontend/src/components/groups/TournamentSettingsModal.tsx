"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@clerk/nextjs";
import { AlertTriangle, CheckCircle, Loader2, Settings, X } from "lucide-react";
import { api } from "@/lib/api";
import type { GroupDetailResponse } from "@/lib/api-types";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

// Matches domain.MinMembersPerGroup / system param group.min_members_for_active
const PREMIUM_MIN_MEMBERS = 5;

type TabId = "settings" | "mode";

function settingsErrorKey(
  error: unknown,
  t: ReturnType<typeof useI18n>["t"],
): string {
  if (!error) return t("group.settingsError");
  const msg = ((error as { message?: string }).message ?? "").toLowerCase();
  if (msg.includes("requires") || msg.includes("members"))
    return t("group.settingsErrorMinMembers").replace(
      "{min}",
      String(PREMIUM_MIN_MEMBERS),
    );
  if (msg.includes("insufficient") || msg.includes("balance"))
    return t("group.settingsErrorBalance");
  return t("group.settingsError");
}

interface Props {
  readonly group: GroupDetailResponse;
  readonly memberCount: number;
  readonly onClose: () => void;
}

export function TournamentSettingsModal({
  group,
  memberCount,
  onClose,
}: Props) {
  const { getToken } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();

  const [activeTab, setActiveTab] = useState<TabId>("settings");

  // ── Tab: Ajustes ──────────────────────────────────────────────────────────
  const [requireApproval, setRequireApproval] = useState(
    group.require_approval,
  );
  const [scoreFromZero, setScoreFromZero] = useState(
    group.score_from_zero,
  );

  const isApprovalDirty = requireApproval !== group.require_approval;
  const isScoreDirty = scoreFromZero !== group.score_from_zero;
  const isSettingsDirty = isApprovalDirty || isScoreDirty;

  const settingsMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      const tasks: Promise<GroupDetailResponse>[] = [];
      if (isApprovalDirty)
        tasks.push(api.updateRequireApproval(token!, group.id, requireApproval));
      if (isScoreDirty)
        tasks.push(api.updateScoreFromZero(token!, group.id, scoreFromZero));
      const results = await Promise.all(tasks);
      return results[results.length - 1]!;
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(["group", group.id], updated);
      onClose();
    },
  });

  // ── Tab: Modo de torneo ───────────────────────────────────────────────────
  const isFree = !group.is_premium;
  const canUpgrade = isFree && memberCount >= PREMIUM_MIN_MEMBERS;
  const modesDisabled = isFree && !canUpgrade;

  const [modeGeneral, setModeGeneral] = useState(group.mode_general);
  const [modeRound, setModeRound] = useState(group.mode_round);

  const modeMutation = useMutation({
    mutationFn: async () => {
      const token = await getToken();
      return api.setTournamentMode(token!, group.id, {
        mode_general: modeGeneral,
        mode_round: modeRound,
      });
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(["group", group.id], updated);
      onClose();
    },
  });

  const isModeDirty =
    modeGeneral !== group.mode_general || modeRound !== group.mode_round;

  const tabs: { id: TabId; label: string }[] = [
    { id: "settings", label: "Ajustes" },
    { id: "mode", label: "Modo de torneo" },
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button
        type="button"
        aria-label="Close"
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative w-full max-w-sm rounded-2xl border border-white/10 bg-[#0b1929] p-6 shadow-2xl">
        {/* Header */}
        <div className="mb-5 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Settings className="h-4 w-4 text-gold-300" />
            <p className="text-sm font-semibold uppercase tracking-wide text-gold-300">
              {t("group.settings")}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 text-text-muted transition-colors hover:text-white"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Tabs */}
        <div className="mb-5 flex gap-1 rounded-lg border border-white/10 bg-white/[0.03] p-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id)}
              className={cn(
                "flex-1 rounded-md px-3 py-1.5 text-xs font-medium transition-colors",
                activeTab === tab.id
                  ? "bg-white/10 text-white"
                  : "text-text-muted hover:text-white",
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab: Ajustes */}
        {activeTab === "settings" && (
          <div>
            <ModeToggle
              id="require-approval"
              checked={requireApproval}
              onChange={setRequireApproval}
              label="Requerir aprobación"
              description={
                requireApproval
                  ? "Los nuevos miembros deben ser aprobados por un miembro activo antes de unirse."
                  : "Los usuarios se unen automáticamente al presentar el código de invitación, sin esperar aprobación."
              }
              disabled={false}
            />

            <div className="mt-4">
              <ModeToggle
                id="score-from-zero"
                checked={scoreFromZero}
                onChange={setScoreFromZero}
                label="Iniciar con 0 puntos"
                description={
                  scoreFromZero
                    ? "Los nuevos miembros que se unan a partir de ahora sólo acumulan puntos de partidos que inicien después de su ingreso."
                    : "Los miembros acumulan todos sus puntos generales del torneo en este grupo."
                }
                disabled={false}
              />
            </div>

            {settingsMutation.isError && (
              <p className="mt-3 rounded-lg border border-red-400/20 bg-red-400/5 px-3 py-2 text-[11px] text-red-400">
                {t("group.settingsError")}
              </p>
            )}

            <button
              type="button"
              disabled={!isSettingsDirty || settingsMutation.isPending}
              onClick={() => settingsMutation.mutate()}
              className={cn(
                "mt-5 flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium transition-colors",
                isSettingsDirty
                  ? "bg-gold-400/80 text-bg-base hover:bg-gold-400 disabled:opacity-50"
                  : "cursor-not-allowed bg-white/5 text-text-muted",
              )}
            >
              {settingsMutation.isPending && (
                <Loader2 className="h-4 w-4 animate-spin" />
              )}
              {t("common.save")}
            </button>
          </div>
        )}

        {/* Tab: Modo de torneo */}
        {activeTab === "mode" && (
          <div>
            <h2 className="mb-4 text-lg font-semibold text-white">
              {t("group.tournamentMode")}
            </h2>

            {/* Premium eligibility banner */}
            {isFree &&
              (canUpgrade ? (
                <div className="mb-4 flex items-start gap-2.5 rounded-xl border border-gold-400/30 bg-gold-400/5 px-3 py-2.5">
                  <CheckCircle className="mt-0.5 h-4 w-4 shrink-0 text-gold-400" />
                  <p className="text-[11px] leading-relaxed text-gold-300">
                    {t("group.premiumReady")}
                  </p>
                </div>
              ) : (
                <div className="mb-4 flex items-start gap-2.5 rounded-xl border border-yellow-400/20 bg-yellow-400/5 px-3 py-2.5">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-400" />
                  <p className="text-[11px] leading-relaxed text-yellow-300">
                    {t("group.premiumNotReady").replace(
                      "{min}",
                      String(PREMIUM_MIN_MEMBERS),
                    )}
                  </p>
                </div>
              ))}

            <div className="space-y-3">
              <ModeToggle
                id="mode-general"
                checked={modeGeneral}
                onChange={setModeGeneral}
                label={t("group.modeGeneral")}
                description={t("group.modeGeneralDesc")}
                disabled={modesDisabled || (!isFree && group.mode_general)}
              />
              <ModeToggle
                id="mode-round"
                checked={modeRound}
                onChange={setModeRound}
                label={t("group.modeRound")}
                description={t("group.modeRoundDesc")}
                disabled={modesDisabled || (!isFree && group.mode_round)}
              />
            </div>

            {modeMutation.isError && (
              <p className="mt-3 rounded-lg border border-red-400/20 bg-red-400/5 px-3 py-2 text-[11px] text-red-400">
                {settingsErrorKey(modeMutation.error, t)}
              </p>
            )}

            <button
              type="button"
              disabled={!isModeDirty || modesDisabled || modeMutation.isPending}
              onClick={() => modeMutation.mutate()}
              className={cn(
                "mt-5 flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium transition-colors",
                isModeDirty && !modesDisabled
                  ? "bg-gold-400/80 text-bg-base hover:bg-gold-400 disabled:opacity-50"
                  : "cursor-not-allowed bg-white/5 text-text-muted",
              )}
            >
              {modeMutation.isPending && (
                <Loader2 className="h-4 w-4 animate-spin" />
              )}
              {t("common.save")}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

interface ModeToggleProps {
  readonly id: string;
  readonly checked: boolean;
  readonly onChange: (v: boolean) => void;
  readonly label: string;
  readonly description: string;
  readonly disabled: boolean;
}

function ModeToggle({
  id,
  checked,
  onChange,
  label,
  description,
  disabled,
}: ModeToggleProps) {
  return (
    <label
      htmlFor={id}
      aria-label={label}
      className={cn(
        "flex cursor-pointer items-start gap-3 rounded-xl border p-3 transition-colors",
        checked
          ? "border-gold-400/30 bg-gold-400/5"
          : "border-white/10 bg-white/[0.02] hover:border-white/20",
        disabled && "cursor-not-allowed opacity-50",
      )}
    >
      <div className="relative mt-0.5 shrink-0">
        <input
          id={id}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
          className="sr-only"
        />
        <div
          className={cn(
            "h-4 w-4 rounded border-2 transition-colors",
            checked
              ? "border-gold-400 bg-gold-400"
              : "border-white/30 bg-transparent",
          )}
        >
          {checked && (
            <svg
              viewBox="0 0 12 10"
              className="h-full w-full p-0.5 text-bg-base"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <polyline points="1,5 4,9 11,1" />
            </svg>
          )}
        </div>
      </div>
      <div className="min-w-0">
        <p className="text-sm font-medium text-white">{label}</p>
        <p className="mt-0.5 text-[11px] text-text-muted">{description}</p>
      </div>
    </label>
  );
}
