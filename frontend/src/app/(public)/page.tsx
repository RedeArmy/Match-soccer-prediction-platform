"use client";

import Link from "next/link";
import {
  ArrowRight,
  ShieldCheck,
  Trophy,
  Zap,
  Award,
  Globe,
  MapPin,
  Flag,
  CheckCircle2,
  XCircle,
  Star,
  Layers,
  BarChart3,
  Users,
  LayoutDashboard,
} from "lucide-react";
import { useAuth } from "@clerk/nextjs";
import { useI18n } from "@/lib/i18n";

// ── Logged-in overrides ───────────────────────────────────────────────────────
// When the user is already signed in every sign-up/join CTA is replaced with
// dashboard/quiniela links so there are no contradictory "create account" prompts.

const LOGGED_IN = {
  heroPrimary: { href: "/dashboard", icon: LayoutDashboard },
  heroSecondary: { href: "/quinielas" },
  freeModeHref: "/quinielas",
  premModeHref: "/quinielas",
  ctaPrimary: { href: "/dashboard" },
  ctaSecondary: { href: "/quinielas" },
};

// ── Style-only constants (no translatable strings) ────────────────────────────

const SCORING_RULE_META = [
  {
    pointsLabel: "+5–15",
    icon: CheckCircle2,
    iconColor: "text-gold-300",
    border: "border-gold-600/30",
    bg: "bg-gold-600/[0.06]",
    pill: "bg-gold-600/20 text-gold-300",
  },
  {
    pointsLabel: "+2–8",
    icon: CheckCircle2,
    iconColor: "text-green-300",
    border: "border-green-500/20",
    bg: "bg-green-500/[0.04]",
    pill: "bg-green-500/15 text-green-300",
  },
  {
    pointsLabel: "0",
    icon: XCircle,
    iconColor: "text-text-muted",
    border: "border-white/8",
    bg: "bg-white/[0.02]",
    pill: "bg-white/8 text-text-muted",
  },
];

// Real point values from scoring_rules (migrations 000063/000065): every
// phase pays out exact-score and correct-outcome points at escalating
// rates so knockout predictions carry more weight than group-stage ones.
const SCORING_BY_PHASE = [
  { phase: "group_stage", exact: 5, correct: 2 },
  { phase: "round_of_16", exact: 8, correct: 4 },
  { phase: "quarter_final", exact: 10, correct: 5 },
  { phase: "semi_final", exact: 12, correct: 6 },
  { phase: "third_place", exact: 12, correct: 6 },
  { phase: "final", exact: 15, correct: 8 },
] as const;

const MODE_META = [
  {
    tagClass: "bg-blue-500/15 text-blue-300",
    guestHref: "/sign-up",
    btnClass: "btn-ghost w-full py-2.5 text-sm",
    cardClass: "border-white/10",
    highlight: false,
  },
  {
    tagClass: "bg-gold-600/20 text-gold-300 border border-gold-600/35",
    guestHref: "/sign-up",
    btnClass: "btn-gold w-full py-2.5 text-sm",
    cardClass: "border-gold-600/35 bg-gold-600/[0.04]",
    highlight: true,
  },
];

const TECH_HIGHLIGHT_META = [
  { icon: Zap, color: "text-blue-300" },
  { icon: BarChart3, color: "text-green-300" },
  { icon: Award, color: "text-gold-300" },
  { icon: Globe, color: "text-blue-200" },
];

// ── Prediction preview (hero right panel) ────────────────────────────────────

function PredictionPreview() {
  const { t } = useI18n();

  const leaderboardRows = [
    { pos: 1, name: "Carlos M.", pts: 24, you: false },
    { pos: 2, name: "Andrea R.", pts: 21, you: false },
    { pos: 3, name: t("landing.previewYou"), pts: 18, you: true },
    { pos: 4, name: "Diego P.", pts: 15, you: false },
  ];

  return (
    <div className="panel p-5 space-y-5">
      {/* Mini leaderboard */}
      <div className="space-y-1.5">
        <p className="text-[10px] uppercase tracking-widest text-text-muted mb-2">
          {t("landing.previewLeader")}
        </p>
        {leaderboardRows.map(({ pos, name, pts, you }) => (
          <div
            key={pos}
            className={
              you
                ? "flex items-center gap-3 rounded-lg border border-gold-600/30 bg-gold-600/10 px-3 py-2 text-sm"
                : "flex items-center gap-3 rounded-lg bg-white/[0.025] px-3 py-2 text-sm"
            }
          >
            <span
              className={`w-5 shrink-0 text-center text-xs font-bold tabular-nums ${you ? "text-gold-300" : "text-text-muted"}`}
            >
              {pos}
            </span>
            <span
              className={`flex-1 truncate ${you ? "font-semibold text-white" : "text-text-secondary"}`}
            >
              {name}
            </span>
            <span
              className={`font-score text-sm tabular-nums ${you ? "text-gold-300" : "text-text-muted"}`}
            >
              {pts} {t("landing.previewPts")}
            </span>
          </div>
        ))}
      </div>

      <div className="wc26-stripe" />

      {/* Prediction card mock */}
      <div className="space-y-3">
        <p className="text-[10px] uppercase tracking-widest text-text-muted">
          {t("landing.previewLabel")}
        </p>
        <div className="rounded-xl border border-white/10 bg-white/[0.025] p-4">
          <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-2">
            {/* Home team */}
            <div className="flex flex-col items-center gap-1.5">
              <span className="text-3xl leading-none select-none">🇲🇽</span>
              <p className="text-xs font-medium text-white">México</p>
            </div>

            {/* Score inputs */}
            <div className="flex flex-col items-center gap-1">
              <p className="text-[9px] uppercase tracking-widest text-text-muted mb-0.5">
                {t("landing.previewScore")}
              </p>
              <div className="flex items-center gap-1.5">
                <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-gold-600/40 bg-gold-600/10 font-display text-xl text-gold-300">
                  2
                </div>
                <span className="text-sm font-bold text-white/25">–</span>
                <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-white/15 bg-white/[0.04] font-display text-xl text-white">
                  1
                </div>
              </div>
            </div>

            {/* Away team */}
            <div className="flex flex-col items-center gap-1.5">
              <span className="text-3xl leading-none select-none">🇺🇸</span>
              <p className="text-xs font-medium text-white">USA</p>
            </div>
          </div>
        </div>

        {/* Mock CTA */}
        <div className="btn-gold w-full cursor-default py-2.5 text-sm">
          {t("landing.previewSave")}
          <span className="ml-auto rounded-full bg-black/20 px-2 py-0.5 text-xs font-normal">
            {t("landing.previewBadge")}
          </span>
        </div>
      </div>
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function LandingPage() {
  const { t, phaseName } = useI18n();
  const { isSignedIn } = useAuth();
  const HeroPrimaryIcon = LOGGED_IN.heroPrimary.icon;

  const tournamentStats = [
    { value: "48", label: t("landing.statTeams"), icon: Users },
    { value: "104", label: t("landing.statMatches"), icon: Trophy },
    { value: "3", label: t("landing.statCountries"), icon: MapPin },
    { value: t("landing.statMonth"), label: t("landing.statYear"), icon: Flag },
  ];

  const scoringRules = [
    {
      ...SCORING_RULE_META[0],
      label: t("landing.scoreExactLabel"),
      example: t("landing.scoreExactEx"),
    },
    {
      ...SCORING_RULE_META[1],
      label: t("landing.scoreResultLabel"),
      example: t("landing.scoreResultEx"),
    },
    {
      ...SCORING_RULE_META[2],
      label: t("landing.scoreMissLabel"),
      example: t("landing.scoreMissEx"),
    },
  ];

  const modes = [
    {
      ...MODE_META[0],
      name: t("landing.freeModeName"),
      tagLabel: t("landing.freeModeTag"),
      desc: t("landing.freeModeDesc"),
      features: [
        t("landing.freeF1"),
        t("landing.freeF2"),
        t("landing.freeF3"),
        t("landing.freeF4"),
      ],
      cta: isSignedIn ? t("landing.loggedInFreeCta") : t("landing.freeCta"),
      href: isSignedIn ? LOGGED_IN.freeModeHref : MODE_META[0].guestHref,
    },
    {
      ...MODE_META[1],
      name: t("landing.premModeName"),
      tagLabel: t("landing.premModeTag"),
      desc: t("landing.premModeDesc"),
      features: [
        t("landing.premF1"),
        t("landing.premF2"),
        t("landing.premF3"),
        t("landing.premF4"),
      ],
      cta: t("landing.premCta"),
      href: isSignedIn ? LOGGED_IN.premModeHref : MODE_META[1].guestHref,
    },
  ];

  const techHighlights = [
    {
      ...TECH_HIGHLIGHT_META[0],
      title: t("landing.tech1Title"),
      desc: t("landing.tech1Desc"),
    },
    {
      ...TECH_HIGHLIGHT_META[1],
      title: t("landing.tech2Title"),
      desc: t("landing.tech2Desc"),
    },
    {
      ...TECH_HIGHLIGHT_META[2],
      title: t("landing.tech3Title"),
      desc: t("landing.tech3Desc"),
    },
    {
      ...TECH_HIGHLIGHT_META[3],
      title: t("landing.tech4Title"),
      desc: t("landing.tech4Desc"),
    },
  ];

  const knockoutPhases = [
    { label: t("landing.ko32"), matches: 16 },
    { label: t("landing.ko16"), matches: 8 },
    { label: t("landing.koQF"), matches: 4 },
    { label: t("landing.koSF"), matches: 2 },
    { label: t("landing.koF"), matches: 1 },
  ];

  const proofPoints = [
    t("landing.proof1"),
    t("landing.proof2"),
    t("landing.proof3"),
  ];

  return (
    <div className="flex flex-col">
      {/* ── Hero ─────────────────────────────────────────────────────────── */}
      <section className="hero-bg relative min-h-[82dvh] px-4 py-16">
        <div className="relative z-10 mx-auto grid max-w-7xl items-start gap-10 lg:grid-cols-[1.1fr_0.9fr]">
          {/* Left: copy + CTAs */}
          <div className="space-y-7 pt-4">
            <div className="inline-flex items-center gap-2 rounded border border-white/10 bg-white/[0.04] px-3 py-1.5 text-xs uppercase tracking-wide text-text-secondary">
              <ShieldCheck className="h-3.5 w-3.5 text-green-300" />
              {t("landing.eyebrow")}
            </div>

            <div className="space-y-4">
              <h1 className="max-w-xl font-display text-6xl leading-[0.92] text-white sm:text-8xl">
                {t("landing.heroLine1")}
                <br />
                <span className="text-gold-300">{t("landing.heroGold")}</span>
                <br />
                {t("landing.heroLine3")}
              </h1>
              <p className="max-w-lg text-balance text-lg text-text-secondary sm:text-xl">
                {t("landing.subtitle")}
              </p>
            </div>

            <div className="flex flex-col gap-3 sm:flex-row">
              {isSignedIn ? (
                <>
                  <Link
                    href={LOGGED_IN.heroPrimary.href}
                    className="btn-gold px-7 py-3 text-base"
                  >
                    <HeroPrimaryIcon className="h-4 w-4" />
                    {t("landing.loggedInHeroPrimary")}
                  </Link>
                  <Link
                    href={LOGGED_IN.heroSecondary.href}
                    className="btn-ghost px-7 py-3 text-base"
                  >
                    {t("landing.loggedInHeroSecondary")}
                  </Link>
                </>
              ) : (
                <>
                  <Link
                    href="/sign-up"
                    className="btn-gold px-7 py-3 text-base"
                  >
                    {t("landing.primary")}
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                  <Link
                    href="/tournaments"
                    className="btn-ghost px-7 py-3 text-base"
                  >
                    {t("landing.secondary")}
                  </Link>
                </>
              )}
            </div>

            {/* Quick proof points */}
            <div className="flex flex-wrap gap-4 pt-1">
              {proofPoints.map((label) => (
                <span
                  key={label}
                  className="flex items-center gap-1.5 text-sm text-text-secondary"
                >
                  <CheckCircle2 className="h-3.5 w-3.5 text-green-400 shrink-0" />
                  {label}
                </span>
              ))}
            </div>
          </div>

          {/* Right: product preview */}
          <div className="lg:pt-4">
            <PredictionPreview />
          </div>
        </div>
      </section>

      {/* ── Tournament stats strip ─────────────────────────────────────── */}
      <section className="border-y border-white/8 bg-white/[0.015] px-4 py-6">
        <div className="mx-auto grid max-w-7xl grid-cols-2 gap-4 sm:grid-cols-4">
          {tournamentStats.map(({ value, label, icon: Icon }) => (
            <div
              key={label}
              className="flex flex-col items-center gap-1 text-center"
            >
              <Icon className="mb-1 h-5 w-5 text-gold-400 opacity-70" />
              <span className="font-display text-4xl text-white">{value}</span>
              <span className="text-xs text-text-muted uppercase tracking-wide">
                {label}
              </span>
            </div>
          ))}
        </div>
      </section>

      {/* ── Scoring rules ─────────────────────────────────────────────── */}
      <section className="px-4 py-16">
        <div className="mx-auto max-w-7xl">
          <div className="mb-10 text-center">
            <p className="text-xs uppercase tracking-widest text-gold-300">
              {t("landing.scoringEyebrow")}
            </p>
            <h2 className="mt-2 font-display text-4xl text-white sm:text-5xl">
              {t("landing.scoringTitle")}
            </h2>
            <p className="mt-3 text-text-secondary max-w-lg mx-auto">
              {t("landing.scoringSubtitle")}
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            {scoringRules.map(
              ({
                pointsLabel,
                label,
                example,
                icon: Icon,
                iconColor,
                border,
                bg,
                pill,
              }) => (
                <div
                  key={label}
                  className={`card p-6 border ${border} ${bg} flex flex-col gap-4`}
                >
                  <div className="flex items-start justify-between">
                    <Icon className={`h-6 w-6 ${iconColor}`} />
                    <span
                      className={`rounded-full px-2.5 py-1 text-sm font-bold tabular-nums ${pill}`}
                    >
                      {pointsLabel} pts
                    </span>
                  </div>
                  <div>
                    <p className="font-semibold text-white">{label}</p>
                    <p className="mt-1 text-sm text-text-muted">{example}</p>
                  </div>
                </div>
              ),
            )}
          </div>

          {/* Per-phase point table — real values from scoring_rules */}
          <div className="mt-8 overflow-x-auto">
            <table className="w-full min-w-[480px] text-sm">
              <thead>
                <tr className="border-b border-white/10 text-left text-xs uppercase tracking-wide text-text-muted">
                  <th className="py-2 pr-4 font-medium">
                    {t("landing.scoringTablePhase")}
                  </th>
                  <th className="py-2 pr-4 text-right font-medium">
                    {t("landing.scoreExactLabel")}
                  </th>
                  <th className="py-2 text-right font-medium">
                    {t("landing.scoreResultLabel")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {SCORING_BY_PHASE.map(({ phase, exact, correct }) => (
                  <tr
                    key={phase}
                    className="border-b border-white/5 last:border-0"
                  >
                    <td className="py-2 pr-4 text-text-secondary">
                      {phaseName(phase)}
                    </td>
                    <td className="py-2 pr-4 text-right font-semibold tabular-nums text-gold-300">
                      +{exact}
                    </td>
                    <td className="py-2 text-right font-semibold tabular-nums text-green-300">
                      +{correct}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Bonus callout */}
          <div className="mt-6 flex items-start gap-3 rounded-xl border border-gold-600/20 bg-gold-600/[0.05] p-4">
            <Star className="h-4 w-4 text-gold-400 mt-0.5 shrink-0" />
            <p className="text-sm text-text-secondary">
              <span className="font-semibold text-gold-300">
                {t("landing.bonusLabel")}
              </span>{" "}
              {t("landing.bonusCopy")}
            </p>
          </div>
        </div>
      </section>

      {/* ── Tournament format ──────────────────────────────────────────── */}
      <section className="px-4 py-16 bg-white/[0.012]">
        <div className="mx-auto max-w-7xl">
          <div className="mb-10">
            <p className="text-xs uppercase tracking-widest text-gold-300">
              {t("landing.formatEyebrow")}
            </p>
            <h2 className="mt-2 font-display text-4xl text-white sm:text-5xl">
              {t("landing.formatTitle")}
            </h2>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            {/* Group stage */}
            <div className="card p-6 space-y-4">
              <div className="flex items-center gap-3">
                <Layers className="h-5 w-5 text-blue-300" />
                <h3 className="font-semibold text-white">
                  {t("landing.groupsTitle")}
                </h3>
              </div>
              <p className="text-sm text-text-secondary">
                {t("landing.groupsDesc")}
              </p>
              <div className="grid grid-cols-3 gap-2 pt-1">
                {[
                  { label: t("landing.groupsGroupsLabel"), value: "12" },
                  { label: t("landing.groupsMatchesLabel"), value: "48" },
                  { label: t("landing.groupsAdvanceLabel"), value: "32" },
                ].map(({ label, value }) => (
                  <div
                    key={label}
                    className="rounded-lg border border-white/8 bg-white/[0.03] p-3 text-center"
                  >
                    <p className="font-display text-2xl text-white">{value}</p>
                    <p className="text-[10px] text-text-muted uppercase tracking-wide mt-0.5">
                      {label}
                    </p>
                  </div>
                ))}
              </div>
            </div>

            {/* Knockout */}
            <div className="card p-6 space-y-4">
              <div className="flex items-center gap-3">
                <Trophy className="h-5 w-5 text-gold-300" />
                <h3 className="font-semibold text-white">
                  {t("landing.knockoutTitle")}
                </h3>
              </div>
              <p className="text-sm text-text-secondary">
                {t("landing.knockoutDesc")}
              </p>
              <div className="space-y-1.5 pt-1">
                {knockoutPhases.map(({ label, matches }, i) => (
                  <div
                    key={label}
                    className="flex items-center justify-between"
                  >
                    <div className="flex items-center gap-2">
                      {/* Funnel visual */}
                      <div
                        className="h-2 rounded-full bg-gold-600/40"
                        style={{ width: `${Math.max(24, 96 - i * 16)}px` }}
                      />
                      <span className="text-sm text-text-secondary">
                        {label}
                      </span>
                    </div>
                    <span className="text-xs tabular-nums text-text-muted">
                      {matches}{" "}
                      {matches === 1
                        ? t("landing.matchSingular")
                        : t("landing.matchPlural")}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* ── Quiniela modes ────────────────────────────────────────────── */}
      <section className="px-4 py-16">
        <div className="mx-auto max-w-7xl">
          <div className="mb-10 text-center">
            <p className="text-xs uppercase tracking-widest text-gold-300">
              {t("landing.modesEyebrow")}
            </p>
            <h2 className="mt-2 font-display text-4xl text-white sm:text-5xl">
              {t("landing.modesTitle")}
            </h2>
          </div>

          <div className="grid gap-6 sm:grid-cols-2 max-w-3xl mx-auto">
            {modes.map(
              ({
                name,
                tagLabel,
                tagClass,
                desc,
                features,
                cta,
                href,
                btnClass,
                cardClass,
                highlight,
              }) => (
                <div
                  key={name}
                  className={`card p-6 border ${cardClass} flex flex-col gap-5 relative`}
                >
                  {highlight && (
                    <div className="absolute -top-px left-1/2 -translate-x-1/2">
                      <div className="h-px w-24 bg-gold-gradient" />
                    </div>
                  )}

                  <div className="flex items-start justify-between">
                    <h3 className="font-display text-3xl text-white">{name}</h3>
                    <span
                      className={`rounded-full px-2.5 py-1 text-xs font-semibold ${tagClass}`}
                    >
                      {tagLabel}
                    </span>
                  </div>

                  <p className="text-sm text-text-secondary">{desc}</p>

                  <ul className="space-y-2 flex-1">
                    {features.map((f) => (
                      <li
                        key={f}
                        className="flex items-center gap-2 text-sm text-text-secondary"
                      >
                        <CheckCircle2 className="h-3.5 w-3.5 text-green-400 shrink-0" />
                        {f}
                      </li>
                    ))}
                  </ul>

                  <Link href={href} className={btnClass}>
                    {cta}
                    <ArrowRight className="h-3.5 w-3.5" />
                  </Link>
                </div>
              ),
            )}
          </div>
        </div>
      </section>

      {/* ── Tech highlights ───────────────────────────────────────────── */}
      <section className="px-4 py-16 bg-white/[0.012]">
        <div className="mx-auto max-w-7xl">
          <div className="mb-10">
            <p className="text-xs uppercase tracking-widest text-gold-300">
              {t("landing.techEyebrow")}
            </p>
            <h2 className="mt-2 font-display text-4xl text-white sm:text-5xl">
              {t("landing.techTitle")}
            </h2>
          </div>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {techHighlights.map(({ icon: Icon, title, desc, color }) => (
              <article key={title} className="card p-5 space-y-3">
                <Icon className={`h-5 w-5 ${color}`} />
                <div>
                  <h3 className="font-semibold text-white text-sm">{title}</h3>
                  <p className="mt-1 text-xs text-text-muted leading-relaxed">
                    {desc}
                  </p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </section>

      {/* ── Final CTA ─────────────────────────────────────────────────── */}
      <section className="px-4 py-20">
        <div className="mx-auto max-w-2xl text-center space-y-6">
          <div className="wc26-stripe mx-auto w-16" />
          <h2 className="font-display text-5xl text-white sm:text-6xl">
            {isSignedIn
              ? t("landing.loggedInCtaTitle1")
              : t("landing.ctaTitle1")}
            <br />
            <span className="text-gold-300">
              {isSignedIn ? t("landing.loggedInCtaGold") : t("landing.ctaGold")}
            </span>
          </h2>
          <p className="text-text-secondary text-lg">
            {isSignedIn
              ? t("landing.loggedInCtaSubtitle")
              : t("landing.ctaSubtitle")}
          </p>
          <div className="flex flex-col sm:flex-row gap-3 justify-center">
            {isSignedIn ? (
              <>
                <Link
                  href={LOGGED_IN.ctaPrimary.href}
                  className="btn-gold px-8 py-3 text-base"
                >
                  <LayoutDashboard className="h-4 w-4" />
                  {t("landing.loggedInCtaPrimary")}
                </Link>
                <Link
                  href={LOGGED_IN.ctaSecondary.href}
                  className="btn-ghost px-8 py-3 text-base"
                >
                  {t("landing.loggedInCtaSecondary")}
                </Link>
              </>
            ) : (
              <>
                <Link href="/sign-up" className="btn-gold px-8 py-3 text-base">
                  {t("landing.ctaPrimary")}
                  <ArrowRight className="h-4 w-4" />
                </Link>
                <Link
                  href="/tournaments"
                  className="btn-ghost px-8 py-3 text-base"
                >
                  {t("landing.ctaSecondary")}
                </Link>
              </>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
