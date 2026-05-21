"use client";

import { useEffect, useMemo, useState } from "react";

type AuditLog = {
  id: string;
  userId?: string;
  action: string;
  targetType?: string;
  targetId?: string;
  ipAddress?: string;
  createdAt: string;
  metadata?: unknown;
};

type Filters = {
  action: string;
  userId: string;
  from: string;
  to: string;
  limit: number;
};

const defaultFilters: Filters = {
  action: "",
  userId: "",
  from: "",
  to: "",
  limit: 200,
};

export default function AdminLogsPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [filters, setFilters] = useState<Filters>(defaultFilters);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const queryString = useMemo(() => {
    const params = new URLSearchParams();
    if (filters.action.trim()) params.set("action", filters.action.trim());
    if (filters.userId.trim()) params.set("userId", filters.userId.trim());
    if (filters.from.trim()) params.set("from", new Date(filters.from).toISOString());
    if (filters.to.trim()) params.set("to", new Date(filters.to).toISOString());
    params.set("limit", String(filters.limit));
    return params.toString();
  }, [filters]);

  async function load() {
    setError(null);
    setLoading(true);
    try {
      const response = await fetch(`/api/admin/audit-logs?${queryString}`, { credentials: "include" });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to load logs");
      setLogs((data.items ?? []) as AuditLog[]);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load logs");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold">Audit Logs</h1>
      <div className="panel p-5 space-y-3">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-5">
          <input
            className="input"
            placeholder="Action"
            value={filters.action}
            onChange={(e) => setFilters((prev) => ({ ...prev, action: e.target.value }))}
          />
          <input
            className="input"
            placeholder="User ID"
            value={filters.userId}
            onChange={(e) => setFilters((prev) => ({ ...prev, userId: e.target.value }))}
          />
          <input
            className="input"
            type="datetime-local"
            value={filters.from}
            onChange={(e) => setFilters((prev) => ({ ...prev, from: e.target.value }))}
          />
          <input
            className="input"
            type="datetime-local"
            value={filters.to}
            onChange={(e) => setFilters((prev) => ({ ...prev, to: e.target.value }))}
          />
          <select
            className="input"
            value={filters.limit}
            onChange={(e) => setFilters((prev) => ({ ...prev, limit: Number(e.target.value) }))}
          >
            <option value={100}>100</option>
            <option value={200}>200</option>
            <option value={500}>500</option>
          </select>
        </div>
        <div className="flex gap-2">
          <button className="btn-primary" type="button" onClick={() => void load()}>
            Apply Filters
          </button>
          <button
            className="btn-ghost"
            type="button"
            onClick={() => {
              setFilters(defaultFilters);
              void (async () => {
                setLogs([]);
                await new Promise((resolve) => setTimeout(resolve, 0));
                const response = await fetch("/api/admin/audit-logs?limit=200", { credentials: "include" });
                const data = await response.json().catch(() => ({}));
                if (!response.ok) {
                  setError(data?.error ?? "Failed to load logs");
                  return;
                }
                setError(null);
                setLogs((data.items ?? []) as AuditLog[]);
              })();
            }}
          >
            Reset
          </button>
        </div>
      </div>

      {error ? <p className="text-sm text-red-600">{error}</p> : null}
      {loading ? <p>Loading logs...</p> : null}

      <div className="panel overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 text-slate-600">
              <th className="px-2 py-2">Time</th>
              <th className="px-2 py-2">Action</th>
              <th className="px-2 py-2">User</th>
              <th className="px-2 py-2">Target</th>
              <th className="px-2 py-2">IP</th>
              <th className="px-2 py-2">Metadata</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((log) => (
              <tr key={log.id} className="border-b border-slate-100 align-top">
                <td className="px-2 py-2 whitespace-nowrap">{new Date(log.createdAt).toLocaleString()}</td>
                <td className="px-2 py-2">{log.action}</td>
                <td className="px-2 py-2">{log.userId ?? "-"}</td>
                <td className="px-2 py-2">
                  {log.targetType ?? "-"}
                  {log.targetId ? <div className="text-xs text-slate-500">{log.targetId}</div> : null}
                </td>
                <td className="px-2 py-2">{log.ipAddress ?? "-"}</td>
                <td className="px-2 py-2 text-xs text-slate-600">
                  <pre className="max-w-xs overflow-auto whitespace-pre-wrap">
                    {log.metadata ? JSON.stringify(log.metadata, null, 2) : "-"}
                  </pre>
                </td>
              </tr>
            ))}
            {!loading && logs.length === 0 ? (
              <tr>
                <td className="px-2 py-4 text-slate-500" colSpan={6}>
                  No logs found for selected filters.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
