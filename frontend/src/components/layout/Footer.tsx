"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAuth } from "@clerk/nextjs";
import { useI18n } from "@/lib/i18n";

export function Footer() {
  const { t } = useI18n();
  const { isSignedIn, isLoaded } = useAuth();
  const pathname = usePathname();

  const authenticated = isLoaded && isSignedIn;
  const isHome = pathname === "/";

  return (
    <footer className="mt-auto border-t border-white/10 bg-[#070A0F]">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <div className="wc26-stripe mb-6" />
        <div className="flex flex-col items-center justify-between gap-4 md:flex-row">
          <div className="flex items-center gap-2">
            <span className="grid h-8 w-8 place-items-center rounded border border-gold-400/40 bg-gold-400/10 font-display text-lg text-gold-300">
              K26
            </span>
            <span className="text-sm text-text-muted">{t("common.event")}</span>
          </div>

          <nav className="flex flex-wrap justify-center gap-x-6 gap-y-2 text-sm text-text-muted">
            {authenticated ? (
              <>
                <Link
                  href="/"
                  className="transition-colors hover:text-text-secondary"
                >
                  {t("common.goHome")}
                </Link>
                <Link
                  href="/dashboard"
                  className="transition-colors hover:text-text-secondary"
                >
                  {t("common.dashboard")}
                </Link>
                <Link
                  href="/quinielas"
                  className="transition-colors hover:text-text-secondary"
                >
                  {t("common.kinielas")}
                </Link>
              </>
            ) : (
              <>
                {isHome ? (
                  <Link
                    href="/tournaments"
                    className="transition-colors hover:text-text-secondary"
                  >
                    {t("common.tournaments")}
                  </Link>
                ) : (
                  <Link
                    href="/"
                    className="transition-colors hover:text-text-secondary"
                  >
                    {t("common.goHome")}
                  </Link>
                )}
                <Link
                  href="/sign-in"
                  className="transition-colors hover:text-text-secondary"
                >
                  {t("common.signIn")}
                </Link>
                <Link
                  href="/sign-up"
                  className="transition-colors hover:text-text-secondary"
                >
                  {t("common.signUp")}
                </Link>
              </>
            )}
          </nav>

          <div className="flex flex-col items-center gap-1 md:items-end">
            <p className="max-w-xs text-center text-xs text-text-muted md:text-right">
              {t("common.responsible")}
            </p>
            <p className="text-xs text-text-muted">
              {t("common.createdBy")}{" "}
              <a
                href="https://github.com/RedeArmy"
                target="_blank"
                rel="noopener noreferrer"
                className="text-gold-400/70 transition-colors hover:text-gold-300"
              >
                @RedeArmy
              </a>
            </p>
          </div>
        </div>
      </div>
    </footer>
  );
}
