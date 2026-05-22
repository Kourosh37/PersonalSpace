"use client";

import { useCallback, useMemo, useState } from "react";

type DownloadState = "queued" | "downloading" | "completed" | "failed" | "canceled" | "native";

export type InAppDownloadRequest = {
  id?: string;
  url: string;
  filename: string;
  sizeBytes?: number;
  credentials?: RequestCredentials;
  forceNative?: boolean;
};

export type DownloadTask = {
  id: string;
  name: string;
  state: DownloadState;
  downloadedBytes: number;
  totalBytes?: number;
  speedBps?: number;
  etaSeconds?: number;
  message?: string;
};

const IN_APP_MAX_BYTES = 200 * 1024 * 1024;

type InternalTask = DownloadTask & { abort?: AbortController };

function formatBytes(value?: number) {
  if (!value || value <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let idx = 0;
  while (current >= 1024 && idx < units.length - 1) {
    current /= 1024;
    idx += 1;
  }
  return `${current.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`;
}

function formatDuration(seconds?: number) {
  if (!seconds || seconds <= 0 || !Number.isFinite(seconds)) return "-";
  const rounded = Math.round(seconds);
  const m = Math.floor(rounded / 60);
  const s = rounded % 60;
  if (m <= 0) return `${s}s`;
  return `${m}m ${s}s`;
}

function supportsInAppDownload() {
  return typeof window !== "undefined" && typeof window.fetch === "function" && typeof window.ReadableStream !== "undefined";
}

function triggerNativeDownload(url: string, filename: string) {
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.rel = "noreferrer";
  anchor.style.display = "none";
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
}

function parseErrorMessage(text: string) {
  try {
    const parsed = JSON.parse(text) as { error?: string };
    if (parsed?.error) return parsed.error;
  } catch {
    // ignore parse failures
  }
  return text || "Download failed";
}

export function useInAppDownloads() {
  const [tasks, setTasks] = useState<InternalTask[]>([]);

  const updateTask = useCallback((id: string, patch: Partial<InternalTask>) => {
    setTasks((prev) => prev.map((task) => (task.id === id ? { ...task, ...patch } : task)));
  }, []);

  const startDownload = useCallback(
    async (request: InAppDownloadRequest) => {
      const id = request.id ? `${request.id}-${Date.now()}` : `download-${Date.now()}`;
      const initialTask: InternalTask = {
        id,
        name: request.filename,
        state: "queued",
        downloadedBytes: 0,
        totalBytes: request.sizeBytes,
      };
      setTasks((prev) => [initialTask, ...prev].slice(0, 12));

      const forceNative =
        request.forceNative ||
        !supportsInAppDownload() ||
        (typeof request.sizeBytes === "number" && request.sizeBytes > IN_APP_MAX_BYTES);

      if (forceNative) {
        triggerNativeDownload(request.url, request.filename);
        updateTask(id, {
          state: "native",
          message:
            request.sizeBytes && request.sizeBytes > IN_APP_MAX_BYTES
              ? "Large file: started in browser native download mode."
              : "Started in browser native download mode.",
        });
        return;
      }

      const controller = new AbortController();
      updateTask(id, { state: "downloading", abort: controller });

      try {
        const response = await fetch(request.url, {
          method: "GET",
          credentials: request.credentials ?? "include",
          signal: controller.signal,
        });
        if (!response.ok) {
          const body = await response.text().catch(() => "");
          throw new Error(parseErrorMessage(body));
        }

        const contentLengthHeader = response.headers.get("Content-Length");
        const headerSize = contentLengthHeader ? Number.parseInt(contentLengthHeader, 10) : undefined;
        const total = Number.isFinite(headerSize) && headerSize && headerSize > 0 ? headerSize : request.sizeBytes;
        updateTask(id, { totalBytes: total });

        if (!response.body) {
          triggerNativeDownload(request.url, request.filename);
          updateTask(id, { state: "native", message: "Streaming API unavailable. Started native download." });
          return;
        }

        const reader = response.body.getReader();
        const chunks: Uint8Array[] = [];
        let downloaded = 0;
        const startedAt = performance.now();

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          if (!value) continue;
          chunks.push(value);
          downloaded += value.byteLength;
          const elapsedSec = Math.max((performance.now() - startedAt) / 1000, 0.001);
          const speed = downloaded / elapsedSec;
          const eta = total && speed > 0 ? (total - downloaded) / speed : undefined;
          updateTask(id, { downloadedBytes: downloaded, speedBps: speed, etaSeconds: eta });
        }

        const blob = new Blob(chunks, {
          type: response.headers.get("Content-Type") || "application/octet-stream",
        });
        const objectUrl = URL.createObjectURL(blob);
        const anchor = document.createElement("a");
        anchor.href = objectUrl;
        anchor.download = request.filename;
        anchor.style.display = "none";
        document.body.appendChild(anchor);
        anchor.click();
        document.body.removeChild(anchor);
        window.setTimeout(() => URL.revokeObjectURL(objectUrl), 15_000);

        updateTask(id, {
          state: "completed",
          downloadedBytes: downloaded,
          totalBytes: total ?? downloaded,
          speedBps: undefined,
          etaSeconds: 0,
          abort: undefined,
        });
      } catch (error) {
        if (controller.signal.aborted) {
          updateTask(id, {
            state: "canceled",
            message: "Canceled by user.",
            abort: undefined,
            speedBps: undefined,
            etaSeconds: undefined,
          });
          return;
        }
        updateTask(id, {
          state: "failed",
          message: error instanceof Error ? error.message : "Download failed",
          abort: undefined,
          speedBps: undefined,
          etaSeconds: undefined,
        });
      }
    },
    [updateTask]
  );

  const cancelDownload = useCallback((id: string) => {
    const task = tasks.find((entry) => entry.id === id);
    task?.abort?.abort();
  }, [tasks]);

  const clearFinished = useCallback(() => {
    setTasks((prev) => prev.filter((task) => task.state === "queued" || task.state === "downloading"));
  }, []);

  const viewTasks = useMemo(
    () =>
      tasks.map((task) => ({
        id: task.id,
        name: task.name,
        state: task.state,
        downloadedBytes: task.downloadedBytes,
        totalBytes: task.totalBytes,
        speedBps: task.speedBps,
        etaSeconds: task.etaSeconds,
        message: task.message,
      })),
    [tasks]
  );

  return {
    tasks: viewTasks,
    startDownload,
    cancelDownload,
    clearFinished,
  };
}

export function InAppDownloadPanel({
  tasks,
  onCancel,
  onClearFinished,
}: {
  tasks: DownloadTask[];
  onCancel: (id: string) => void;
  onClearFinished: () => void;
}) {
  if (tasks.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-50 w-full max-w-md rounded-2xl border border-slate-200 bg-white p-3 shadow-xl">
      <div className="mb-2 flex items-center justify-between">
        <p className="text-sm font-semibold">Downloads</p>
        <button className="btn-ghost !px-2 !py-1 text-xs" onClick={onClearFinished} type="button">
          Clear Finished
        </button>
      </div>

      <div className="max-h-80 space-y-2 overflow-y-auto pr-1">
        {tasks.map((task) => {
          const pct =
            task.totalBytes && task.totalBytes > 0 ? Math.min(100, Math.max(0, (task.downloadedBytes / task.totalBytes) * 100)) : undefined;

          return (
            <div className="rounded-xl border border-slate-200 p-2" key={task.id}>
              <div className="flex items-center justify-between gap-2">
                <p className="truncate text-xs font-medium">{task.name}</p>
                {task.state === "downloading" ? (
                  <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => onCancel(task.id)} type="button">
                    Cancel
                  </button>
                ) : null}
              </div>
              <p className="mt-1 text-xs text-slate-600">
                {task.state} | {formatBytes(task.downloadedBytes)}
                {task.totalBytes ? ` / ${formatBytes(task.totalBytes)}` : ""}
              </p>
              {task.state === "downloading" ? (
                <p className="text-xs text-slate-500">
                  speed: {formatBytes(task.speedBps)}/s | eta: {formatDuration(task.etaSeconds)}
                </p>
              ) : null}
              {typeof pct === "number" ? (
                <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-200">
                  <div className="h-full bg-pine" style={{ width: `${pct}%` }} />
                </div>
              ) : null}
              {task.message ? <p className="mt-1 text-xs text-slate-600">{task.message}</p> : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}
