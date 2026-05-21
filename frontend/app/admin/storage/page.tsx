"use client";

import { useEffect, useState } from "react";

type Summary = {
  fileCount: number;
  folderCount: number;
  totalUsedStorageBytes: number;
  incompleteUploadBytes: number;
  storageRoot: string;
};

function formatBytes(value: number) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let i = 0;
  while (current >= 1024 && i < units.length - 1) {
    current /= 1024;
    i += 1;
  }
  return `${current.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export default function AdminStoragePage() {
  const [summary, setSummary] = useState<Summary | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    try {
      const response = await fetch("/api/admin/storage/summary", { credentials: "include" });
      const data = await response.json();
      if (!response.ok) throw new Error(data?.error ?? "Failed to load storage summary");
      setSummary(data as Summary);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load storage summary");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function recalc() {
    setError(null);
    try {
      const response = await fetch("/api/admin/storage/recalculate", {
        method: "POST",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to recalculate");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to recalculate");
    }
  }

  async function cleanupExpired() {
    setError(null);
    try {
      const response = await fetch("/api/admin/storage/cleanup-expired-uploads", {
        method: "POST",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to cleanup expired uploads");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to cleanup expired uploads");
    }
  }

  async function cleanupPreviewCache() {
    setError(null);
    try {
      const response = await fetch("/api/admin/storage/cleanup-preview-cache", {
        method: "POST",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to cleanup preview cache");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to cleanup preview cache");
    }
  }

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold">Storage</h1>
      {error ? <p className="text-sm text-red-600">{error}</p> : null}

      <div className="panel p-5 space-y-2">
        {summary ? (
          <>
            <p>Files: {summary.fileCount}</p>
            <p>Folders: {summary.folderCount}</p>
            <p>Total used: {formatBytes(summary.totalUsedStorageBytes)}</p>
            <p>Incomplete uploads: {formatBytes(summary.incompleteUploadBytes)}</p>
            <p>Storage root: {summary.storageRoot}</p>
          </>
        ) : (
          <p>Loading...</p>
        )}
      </div>

      <div className="flex gap-3">
        <button className="btn-primary" type="button" onClick={recalc}>
          Recalculate Usage
        </button>
        <button className="btn-ghost" type="button" onClick={cleanupExpired}>
          Cleanup Expired Uploads
        </button>
        <button className="btn-ghost" type="button" onClick={cleanupPreviewCache}>
          Cleanup Preview Cache
        </button>
      </div>
    </div>
  );
}
