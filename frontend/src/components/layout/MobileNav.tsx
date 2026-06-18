"use client";

import Link from "next/link";
import { SignedIn, SignedOut, useClerk } from "@clerk/nextjs";
import {
  Home,
  LayoutDashboard,
  LogOut,
  Trophy,
  Wallet,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { LanguageSwitcher } from "./LanguageSwitcher";

interface MobileNavProps {
  readonly open: boolean;
  readonly onClose: () => void;
}

const navItems = [
  {
    href: "/",
    labelKey: "nav.home",
    icon: Home,
    public: true,
    signedOutOnly: false,
  },
  {
    href: "/tournaments",
    labelKey: "common.tournaments",
    icon: Trophy,
    public: true,
    signedOutOnly: false,
  },
  {
    href: "/quinielas",
    labelKey: "common.kinielas",
    icon: Trophy,
    public: false,
    signedOutOnly: false,
  },
  {
    href: "/dashboard",
    labelKey: "common.dashboard",
    icon: LayoutDashboard,
    public: false,
    signedOutOnly: false,
  },
  {
    href: "/balance",
    labelKey: "common.balance",
    icon: Wallet,
    public: false,
    signedOutOnly: false,
  },
];

export function MobileNav({ open, onClose }: MobileNavProps) {
  const { signOut } = useClerk();
  const { t } = useI18n();

  return (
    <>
      <button
        type="button"
        aria-label={t("common.closeMenu")}
        className={cn(
          "fixed inset-0 z-40 bg-[#070A0F]/80 backdrop-blur-sm transition-opacity md:hidden",
          open
            ? "opacity-100 pointer-events-auto"
            : "opacity-0 pointer-events-none",
        )}
        onClick={onClose}
      />

      <aside
        className={cn(
          "fixed inset-y-0 right-0 z-50 flex w-72 flex-col border-l border-white/10 bg-[#0D1420]",
          "transition-transform duration-300 md:hidden",
          open ? "translate-x-0" : "translate-x-full",
        )}
      >
        <div className="flex items-center justify-between border-b border-white/10 p-4">
          <span className="font-display text-xl text-gold-400">
            {t("common.brand")}
          </span>
          <button
            onClick={onClose}
            className="p-1 text-text-secondary hover:text-text-primary"
            aria-label={t("common.closeMenu")}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="border-b border-white/10 p-4">
          <LanguageSwitcher compact />
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto p-4">
          {navItems.map(
            ({
              href,
              labelKey,
              icon: Icon,
              public: isPublic,
              signedOutOnly,
            }) => (
              <NavLink
                key={href}
                href={href}
                label={t(labelKey)}
                icon={<Icon className="h-4 w-4" />}
                isPublic={isPublic}
                signedOutOnly={signedOutOnly}
                onClose={onClose}
              />
            ),
          )}
        </nav>

        <div className="space-y-2 border-t border-white/10 p-4">
          <SignedOut>
            <Link
              href="/sign-in"
              onClick={onClose}
              className="btn-ghost block w-full py-2 text-center text-sm"
            >
              {t("common.signIn")}
            </Link>
            <Link
              href="/sign-up"
              onClick={onClose}
              className="btn-gold block w-full py-2 text-center text-sm"
            >
              {t("common.signUp")}
            </Link>
          </SignedOut>
          <SignedIn>
            <button
              onClick={() => {
                signOut();
                onClose();
              }}
              className="flex w-full items-center gap-2 rounded px-3 py-2 text-sm text-red-300 transition-colors hover:bg-red-400/10 hover:text-red-200"
            >
              <LogOut className="h-4 w-4" />
              {t("common.signOut")}
            </button>
          </SignedIn>
        </div>
      </aside>
    </>
  );
}

function NavLink({
  href,
  label,
  icon,
  isPublic,
  signedOutOnly,
  onClose,
}: Readonly<{
  href: string;
  label: string;
  icon: React.ReactNode;
  isPublic: boolean;
  signedOutOnly: boolean;
  onClose: () => void;
}>) {
  const content = (
    <Link
      href={href}
      onClick={onClose}
      className="flex items-center gap-3 rounded px-3 py-2.5 text-sm text-text-secondary transition-colors hover:bg-white/[0.05] hover:text-text-primary"
    >
      {icon}
      {label}
    </Link>
  );

  if (signedOutOnly) return <SignedOut>{content}</SignedOut>;
  if (!isPublic) return <SignedIn>{content}</SignedIn>;
  return content;
}
