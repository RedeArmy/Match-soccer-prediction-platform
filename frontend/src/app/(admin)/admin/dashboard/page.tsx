"use client";

import { useAuth } from "@clerk/nextjs";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { LoadingState } from "@/components/shared/LoadingState";
import {
  Users,
  ShieldCheck,
  Wallet,
  TrendingUp,
  Activity,
  AlertCircle,
} from "lucide-react";

export default function AdminDashboardPage() {
  const { getToken } = useAuth();
  const { data: stats, isLoading } = useQuery({
    queryKey: ["admin-stats"],
    queryFn: async () => {
      const token = await getToken();
      return api.adminGetStats(token!);
    },
    refetchInterval: 30_000,
  });

  const kpis = stats
    ? [
        {
          label: "Usuarios totales",
          value: stats.users.total.toLocaleString(),
          icon: Users,
          color: "text-blue-400",
        },
        {
          label: "Usuarios activos",
          value: stats.users.active.toLocaleString(),
          icon: Activity,
          color: "text-green-400",
        },
        {
          label: "Usuarios baneados",
          value: stats.users.banned.toLocaleString(),
          icon: ShieldCheck,
          color: "text-gold-400",
        },
        {
          label: "Grupos activos",
          value: stats.groups.active.toLocaleString(),
          icon: TrendingUp,
          color: "text-maple-400",
        },
        {
          label: "Pagos pendientes",
          value: stats.payments.pending.toLocaleString(),
          icon: Wallet,
          color: "text-red-400",
        },
        {
          label: "Pagos confirmados",
          value: stats.payments.confirmed.toLocaleString(),
          icon: AlertCircle,
          color: "text-green-400",
        },
      ]
    : [];

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">DASHBOARD ADMIN</h1>

      {isLoading ? (
        <LoadingState rows={6} />
      ) : (
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {kpis.map(({ label, value, icon: Icon, color }) => (
            <div key={label} className="card p-5 flex items-center gap-4">
              <div className={`p-2.5 rounded-lg bg-blue-800 ${color}`}>
                <Icon className="w-5 h-5" />
              </div>
              <div>
                <p className="text-xs text-text-muted">{label}</p>
                <p className="font-score text-xl text-white font-semibold">
                  {value}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
