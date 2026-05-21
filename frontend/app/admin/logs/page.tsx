"use client";

import { useEffect, useState } from "react";

type AuditLog = {
  id: string;
  userId?: string;
  action: string;
  targetType?: string;
  targetId?: string;
  ipAddress?: string;
  createdAt: string;
};

export default function AdminLogsPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const response = await fetch("/api/admin/audit-logs?limit=200", { credentials: "include" });
        const data = await response.json();
        if (!response.ok) throw new Error(data?.error ?? "Failed to load logs");
        setLogs(data.items ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load logs");
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  if (loading) return <p>Loading logs...</p>;

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold">Audit Logs</h1>
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
      <div className="panel overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 text-slate-600">
              <th className="px-2 py-2">Time</th>
              <th className="px-2 py-2">Action</th>
              <th className="px-2 py-2">User</th>
              <th className="px-2 py-2">Target</th>
              <th className="px-2 py-2">IP</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((log) => (
              <tr key={log.id} className="border-b border-slate-100">
                <td className="px-2 py-2">{new Date(log.createdAt).toLocaleString()}</td>
                <td className="px-2 py-2">{log.action}</td>
                <td className="px-2 py-2">{log.userId ?? "-"}</td>
                <td className="px-2 py-2">{log.targetType ?? "-"}</td>
                <td className="px-2 py-2">{log.ipAddress ?? "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}