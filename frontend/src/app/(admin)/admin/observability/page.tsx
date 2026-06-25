"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { LoadingState } from "@/components/shared/LoadingState";
import { StatusBadge } from "@/components/shared/StatusBadge";
import {
  Activity,
  Wifi,
  BarChart2,
  FileText,
  LayoutDashboard,
  ExternalLink,
  Search,
  RefreshCw,
  Clock,
} from "lucide-react";

const TABS = [
  { id: "prometheus", label: "Prometheus", icon: BarChart2 },
  { id: "logs", label: "Logs", icon: FileText },
  { id: "grafana", label: "Grafana", icon: LayoutDashboard },
  { id: "sistema", label: "Sistema", icon: Activity },
] as const;

type TabId = (typeof TABS)[number]["id"];

const CB_BADGE_STATUS: Record<string, string> = {
  closed: "approved",
  open: "rejected",
};

const GRAFANA_URL = process.env.NEXT_PUBLIC_GRAFANA_URL ?? "";

const SINCE_OPTIONS = [
  { label: "15 min", value: 15 },
  { label: "1 hora", value: 60 },
  { label: "6 horas", value: 360 },
  { label: "24 horas", value: 1440 },
];

const PRESET_QUERIES = [
  {
    label: "Request rate (req/s)",
    q: `sum(rate(http_server_request_duration_seconds_count[5m]))`,
  },
  {
    label: "Error rate 5xx (%)",
    q: `100 * sum(rate(http_server_request_duration_seconds_count{http_status_code=~"5.."}[5m])) / sum(rate(http_server_request_duration_seconds_count[5m]))`,
  },
  {
    label: "P95 latency (s)",
    q: `histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[5m])))`,
  },
  {
    label: "Goroutines",
    q: `go_goroutines`,
  },
  {
    label: "Heap alloc (MB)",
    q: `go_memstats_heap_alloc_bytes / 1024 / 1024`,
  },
  {
    label: "DB open connections",
    q: `go_sql_open_connections`,
  },
];

function nsToDate(ns: string): string {
  try {
    const ms = Math.floor(Number(ns) / 1_000_000);
    return new Date(ms).toLocaleString("es-GT");
  } catch {
    return "—";
  }
}

export default function AdminObservabilityPage() {
  const { getToken } = useAuth();
  const [activeTab, setActiveTab] = useState<TabId>("prometheus");

  // ── Prometheus tab state ────────────────────────────────────────────────────
  const [customQuery, setCustomQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState<string | null>(null);

  // ── Logs tab state ──────────────────────────────────────────────────────────
  const [logSearch, setLogSearch] = useState("");
  const [pendingSearch, setPendingSearch] = useState("");
  const [since, setSince] = useState(15);

  // ── Data queries ────────────────────────────────────────────────────────────
  const { data: metricsSummary, isLoading: summaryLoading } = useQuery({
    queryKey: ["admin-metrics-summary"],
    queryFn: async () => {
      const token = await getToken();
      return api.adminGetMetricsSummary(token!);
    },
    refetchInterval: 30_000,
    enabled: activeTab === "prometheus",
  });

  const { data: customResult, isFetching: queryFetching } = useQuery({
    queryKey: ["admin-metrics-query", activeQuery],
    queryFn: async () => {
      const token = await getToken();
      return api.adminQueryMetrics(token!, activeQuery!);
    },
    enabled: !!activeQuery && activeTab === "prometheus",
  });

  const {
    data: logsData,
    isLoading: logsLoading,
    refetch: refetchLogs,
  } = useQuery({
    queryKey: ["admin-logs", logSearch, since],
    queryFn: async () => {
      const token = await getToken();
      return api.adminSearchLogs(token!, { q: logSearch || undefined, since });
    },
    refetchInterval: 60_000,
    enabled: activeTab === "logs",
  });

  const { data: sseStats, isLoading: sseLoading } = useQuery({
    queryKey: ["admin-sse-stats"],
    queryFn: async () => {
      const token = await getToken();
      return api.adminGetSSEStats(token!);
    },
    refetchInterval: 15_000,
    enabled: activeTab === "sistema",
  });

  const { data: breakers, isLoading: breakersLoading } = useQuery({
    queryKey: ["admin-circuit-breakers"],
    queryFn: async () => {
      const token = await getToken();
      return api.adminGetCircuitBreakers(token!);
    },
    refetchInterval: 30_000,
    enabled: activeTab === "sistema",
  });

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl text-white">OBSERVABILIDAD</h1>

      {/* Tab bar */}
      <div className="flex gap-1 rounded-xl border border-blue-800/50 bg-blue-950/40 p-1">
        {TABS.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            onClick={() => setActiveTab(id)}
            className={`flex flex-1 items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
              activeTab === id
                ? "bg-blue-700 text-white"
                : "text-text-muted hover:text-text-secondary"
            }`}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span className="hidden sm:inline">{label}</span>
          </button>
        ))}
      </div>

      {/* ── Prometheus ────────────────────────────────────────────────────────── */}
      {activeTab === "prometheus" && (
        <div className="space-y-6">
          {/* Summary cards */}
          <section className="space-y-3">
            <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-text-secondary">
              <BarChart2 className="h-4 w-4 text-gold-400" />
              Resumen (últimos 5 min)
            </h2>
            {summaryLoading && <LoadingState rows={1} />}
            {metricsSummary && !metricsSummary.configured && (
              <p className="text-sm text-text-muted">
                Prometheus no configurado (WCQ_OBSERVABILITY_PROMETHEUSURL).
              </p>
            )}
            {metricsSummary?.configured && (
              <div className="grid gap-4 sm:grid-cols-3">
                {[
                  {
                    label: "Request rate",
                    value: `${metricsSummary.request_rate_per_sec.toFixed(2)} req/s`,
                  },
                  {
                    label: "Error rate 5xx",
                    value: `${(metricsSummary.error_rate_5xx * 100).toFixed(2)}%`,
                  },
                  {
                    label: "P95 latency",
                    value: `${(metricsSummary.p95_latency_seconds * 1000).toFixed(0)} ms`,
                  },
                ].map(({ label, value }) => (
                  <div key={label} className="card p-4">
                    <p className="text-xs text-text-muted">{label}</p>
                    <p className="font-score text-2xl text-white">{value}</p>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* Preset queries */}
          <section className="space-y-3">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
              Consultas rápidas
            </h2>
            <div className="flex flex-wrap gap-2">
              {PRESET_QUERIES.map(({ label, q }) => (
                <button
                  key={label}
                  onClick={() => {
                    setActiveQuery(q);
                    setCustomQuery(q);
                  }}
                  className="rounded-lg border border-blue-700/50 bg-blue-900/40 px-3 py-1.5 text-xs text-text-secondary transition-colors hover:border-blue-500 hover:text-white"
                >
                  {label}
                </button>
              ))}
            </div>
          </section>

          {/* Custom PromQL */}
          <section className="space-y-3">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-text-secondary">
              PromQL personalizado
            </h2>
            <div className="flex gap-2">
              <input
                type="text"
                value={customQuery}
                onChange={(e) => setCustomQuery(e.target.value)}
                onKeyDown={(e) =>
                  e.key === "Enter" && setActiveQuery(customQuery)
                }
                placeholder="sum(rate(http_server_request_duration_seconds_count[5m]))"
                className="flex-1 rounded-lg border border-blue-700/50 bg-blue-950 px-3 py-2 font-mono text-sm text-white placeholder:text-text-muted focus:border-blue-500 focus:outline-none"
              />
              <button
                onClick={() => setActiveQuery(customQuery)}
                disabled={!customQuery || queryFetching}
                className="flex items-center gap-1.5 rounded-lg bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50 hover:bg-blue-600"
              >
                {queryFetching ? (
                  <RefreshCw className="h-4 w-4 animate-spin" />
                ) : (
                  <Search className="h-4 w-4" />
                )}
                Ejecutar
              </button>
            </div>

            {customResult?.configured && customResult.results.length > 0 && (
              <div className="card overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-blue-800/50">
                      <th className="px-4 py-2 text-left font-medium text-text-muted">
                        Labels
                      </th>
                      <th className="px-4 py-2 text-right font-medium text-text-muted">
                        Valor
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-blue-800/30">
                    {customResult.results.map((r, i) => (
                      <tr key={`${Object.values(r.labels).join("-")}-${i}`}>
                        <td className="px-4 py-2 font-mono text-xs text-text-secondary">
                          {Object.entries(r.labels)
                            .filter(([k]) => k !== "__name__")
                            .map(([k, v]) => `${k}="${v}"`)
                            .join(", ") || "—"}
                        </td>
                        <td className="px-4 py-2 text-right font-score text-white">
                          {r.value.toFixed(4)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {customResult?.configured && customResult.results.length === 0 && (
              <p className="text-sm text-text-muted">Sin resultados.</p>
            )}
          </section>
        </div>
      )}

      {/* ── Logs ──────────────────────────────────────────────────────────────── */}
      {activeTab === "logs" && (
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" />
              <input
                type="text"
                value={pendingSearch}
                onChange={(e) => setPendingSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") setLogSearch(pendingSearch);
                }}
                placeholder="grep: filtrar por servicio u operación..."
                className="w-full rounded-lg border border-blue-700/50 bg-blue-950 py-2 pl-9 pr-3 text-sm text-white placeholder:text-text-muted focus:border-blue-500 focus:outline-none"
              />
            </div>
            <div className="flex items-center gap-1 rounded-lg border border-blue-700/50 bg-blue-950 px-2">
              <Clock className="h-4 w-4 text-text-muted" />
              <select
                value={since}
                onChange={(e) => setSince(Number(e.target.value))}
                className="bg-transparent py-2 text-sm text-white focus:outline-none"
              >
                {SINCE_OPTIONS.map(({ label, value }) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            <button
              onClick={() => {
                setLogSearch(pendingSearch);
                refetchLogs();
              }}
              className="flex items-center gap-1.5 rounded-lg bg-blue-700 px-4 py-2 text-sm font-medium text-white hover:bg-blue-600"
            >
              <Search className="h-4 w-4" />
              Buscar
            </button>
          </div>

          {logsLoading && <LoadingState rows={3} />}

          {logsData && !logsData.configured && (
            <p className="text-sm text-text-muted">
              Tempo no configurado (WCQ_OBSERVABILITY_TEMPOURL).
            </p>
          )}

          {logsData?.configured && logsData.errors.length === 0 && (
            <p className="text-sm text-text-muted">
              Sin errores en el período seleccionado.
            </p>
          )}

          {logsData?.configured && logsData.errors.length > 0 && (
            <div className="card overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-blue-800/50">
                    <th className="px-4 py-2 text-left font-medium text-text-muted">
                      Servicio
                    </th>
                    <th className="px-4 py-2 text-left font-medium text-text-muted">
                      Operación
                    </th>
                    <th className="px-4 py-2 text-right font-medium text-text-muted">
                      Duración
                    </th>
                    <th className="px-4 py-2 text-left font-medium text-text-muted">
                      Timestamp
                    </th>
                    <th className="px-4 py-2 text-left font-medium text-text-muted">
                      Trace ID
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-blue-800/30">
                  {logsData.errors.map((e) => (
                    <tr key={e.trace_id} className="hover:bg-blue-900/20">
                      <td className="px-4 py-2 text-xs text-gold-300">
                        {e.root_service_name}
                      </td>
                      <td className="px-4 py-2 font-mono text-xs text-text-primary">
                        {e.root_trace_name}
                      </td>
                      <td className="px-4 py-2 text-right text-xs text-text-secondary">
                        {e.duration_ms} ms
                      </td>
                      <td className="px-4 py-2 text-xs text-text-muted">
                        {nsToDate(e.start_time_unix_nano)}
                      </td>
                      <td className="px-4 py-2">
                        {GRAFANA_URL ? (
                          <a
                            href={`${GRAFANA_URL}/explore?orgId=1&left=%7B"datasource"%3A"tempo"%2C"queries"%3A%5B%7B"refId"%3A"A"%2C"queryType"%3A"traceId"%2C"query"%3A"${e.trace_id}"%7D%5D%7D`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center gap-1 font-mono text-xs text-blue-400 hover:text-blue-300"
                          >
                            {e.trace_id.slice(0, 16)}…
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        ) : (
                          <button
                            type="button"
                            className="cursor-pointer font-mono text-xs text-blue-400 hover:text-blue-300 bg-transparent border-0 p-0"
                            title={e.trace_id}
                            onClick={() =>
                              navigator.clipboard.writeText(e.trace_id)
                            }
                          >
                            {e.trace_id.slice(0, 16)}…
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ── Grafana ───────────────────────────────────────────────────────────── */}
      {activeTab === "grafana" && (
        <div className="space-y-4">
          {GRAFANA_URL ? (
            <>
              <div className="flex items-center justify-between">
                <p className="text-sm text-text-muted">{GRAFANA_URL}</p>
                <a
                  href={GRAFANA_URL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1.5 rounded-lg border border-blue-700/50 px-3 py-1.5 text-sm text-text-secondary hover:text-white"
                >
                  <ExternalLink className="h-4 w-4" />
                  Abrir en nueva pestaña
                </a>
              </div>
              <iframe
                src={GRAFANA_URL}
                className="h-[70vh] w-full rounded-xl border border-blue-800/50"
                title="Grafana"
              />
            </>
          ) : (
            <div className="card p-6 space-y-3">
              <p className="font-medium text-white">Grafana no configurado</p>
              <p className="text-sm text-text-muted">
                Agrega{" "}
                <code className="rounded bg-blue-800 px-1 text-xs">
                  NEXT_PUBLIC_GRAFANA_URL
                </code>{" "}
                como variable en GitHub Actions → Settings → Variables → Actions
                y haz redeploy.
              </p>
              <p className="text-sm text-text-muted">
                Grafana ya está expuesto públicamente vía{" "}
                <code className="rounded bg-blue-800 px-1 text-xs">
                  $GRAFANA_DOMAIN
                </code>{" "}
                en el Caddyfile. El valor debería ser algo como{" "}
                <code className="rounded bg-blue-800 px-1 text-xs">
                  https://grafana.kiniela.uk
                </code>
                .
              </p>
            </div>
          )}
        </div>
      )}

      {/* ── Sistema ───────────────────────────────────────────────────────────── */}
      {activeTab === "sistema" && (
        <div className="space-y-6">
          <section className="space-y-3">
            <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-text-secondary">
              <Wifi className="h-4 w-4 text-blue-400" />
              SSE Hub
            </h2>
            {sseLoading && <LoadingState rows={1} />}
            {sseStats && (
              <div className="grid gap-4 sm:grid-cols-3">
                {[
                  {
                    label: "Usuarios conectados",
                    value: sseStats.connected_users,
                  },
                  {
                    label: "Conexiones totales",
                    value: sseStats.total_connections,
                  },
                  {
                    label: "Eventos descartados",
                    value: sseStats.dropped_events,
                  },
                ].map(({ label, value }) => (
                  <div key={label} className="card p-4">
                    <p className="text-xs text-text-muted">{label}</p>
                    <p className="font-score text-2xl text-white">{value}</p>
                  </div>
                ))}
              </div>
            )}
          </section>

          <section className="space-y-3">
            <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-text-secondary">
              <Activity className="h-4 w-4 text-gold-400" />
              Circuit Breakers
            </h2>
            {breakersLoading && <LoadingState rows={2} />}
            {!breakersLoading && (breakers?.length ?? 0) === 0 && (
              <p className="text-sm text-text-muted">
                Sin circuit breakers registrados.
              </p>
            )}
            {!breakersLoading && (breakers?.length ?? 0) > 0 && (
              <div className="card divide-y divide-blue-800/50">
                {breakers?.map((b) => (
                  <div
                    key={b.name}
                    className="flex items-center justify-between gap-3 px-4 py-3"
                  >
                    <div>
                      <p className="font-mono text-sm text-text-primary">
                        {b.name}
                      </p>
                      <p className="text-xs text-text-muted">
                        Fallos: {b.failures}
                      </p>
                    </div>
                    <StatusBadge
                      status={CB_BADGE_STATUS[b.state] ?? "pending"}
                      size="sm"
                    />
                  </div>
                ))}
              </div>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
