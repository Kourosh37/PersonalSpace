"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Uppy from "@uppy/core";
import Tus from "@uppy/tus";
import Dashboard from "@uppy/dashboard";
import "@uppy/core/css/style.min.css";
import "@uppy/dashboard/css/style.min.css";

type Props = {
  folderId: string | null;
  onComplete?: () => void;
};

type UploaderMeta = {
  folderid: string;
  filename?: string;
  name?: string;
};

type UploadStatus = "queued" | "preparing" | "uploading" | "paused" | "retrying" | "completed" | "failed" | "canceled";

type UploadRow = {
  id: string;
  name: string;
  status: UploadStatus;
  progressPercent: number;
  uploadedBytes: number;
  totalBytes?: number;
  speedBps?: number;
  etaSeconds?: number;
  retries: number;
  message?: string;
  updatedAt: number;
};

type RowOverride = {
  status?: UploadStatus;
  retries?: number;
  message?: string;
};

function formatBytes(value?: number) {
  if (!value || value <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let i = 0;
  while (current >= 1024 && i < units.length - 1) {
    current /= 1024;
    i += 1;
  }
  return `${current.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatDuration(seconds?: number) {
  if (!seconds || seconds <= 0 || !Number.isFinite(seconds)) return "-";
  const rounded = Math.round(seconds);
  const m = Math.floor(rounded / 60);
  const s = rounded % 60;
  if (m <= 0) return `${s}s`;
  return `${m}m ${s}s`;
}

function isFinalStatus(status: UploadStatus) {
  return status === "completed" || status === "failed" || status === "canceled";
}

export function TusUploader({ folderId, onComplete }: Props) {
  const dashboardRef = useRef<HTMLDivElement | null>(null);
  const folderInputRef = useRef<HTMLInputElement | null>(null);
  const [rows, setRows] = useState<UploadRow[]>([]);
  const [isOnline, setIsOnline] = useState(true);
  const [lastResumeHint, setLastResumeHint] = useState<string | null>(null);
  const [folderUploadHint, setFolderUploadHint] = useState<string | null>(null);
  const overridesRef = useRef<Record<string, RowOverride>>({});
  const preparingSetRef = useRef<Set<string>>(new Set());

  const uppy = useMemo(() => {
    const instance = new Uppy<UploaderMeta, Record<string, never>>({
      autoProceed: false,
      restrictions: {
        maxNumberOfFiles: 50,
      },
      allowMultipleUploadBatches: true,
      meta: {
        folderid: folderId ?? "",
      },
    });

    instance.use(Tus, {
      endpoint: "/api/uploads/tus",
      chunkSize: 5 * 1024 * 1024,
      withCredentials: true,
      removeFingerprintOnSuccess: false,
      retryDelays: [0, 1000, 3000, 5000, 10000, 20000],
      allowedMetaFields: ["name", "filename", "folderid"],
      headers: {
        "Tus-Resumable": "1.0.0",
      },
    });

    return instance;
  }, [folderId]);

  const syncRows = useCallback(() => {
    const now = Date.now();
    const activeFiles = uppy.getFiles();

    setRows((prev) => {
      const byId = new Map(prev.map((row) => [row.id, row]));
      const activeIDs = new Set(activeFiles.map((file) => file.id));

      for (const file of activeFiles) {
        const existing = byId.get(file.id);
        const override = overridesRef.current[file.id];
        const progress = file.progress ?? {};
        const uploadedBytes = typeof progress.bytesUploaded === "number" ? progress.bytesUploaded : 0;
        const totalBytes =
          typeof progress.bytesTotal === "number" && progress.bytesTotal > 0
            ? progress.bytesTotal
            : typeof file.size === "number"
              ? file.size
              : undefined;

        let status: UploadStatus;
        if (progress.uploadComplete) {
          status = "completed";
        } else if (!isOnline && typeof progress.uploadStarted === "number" && uploadedBytes > 0) {
          status = "paused";
        } else if (override?.status === "retrying") {
          status = "retrying";
        } else if (file.error) {
          status = "failed";
        } else if (file.isPaused) {
          status = "paused";
        } else if (preparingSetRef.current.has(file.id)) {
          status = "preparing";
        } else if (typeof progress.uploadStarted === "number" && uploadedBytes > 0) {
          status = "uploading";
        } else {
          status = "queued";
        }

        let speedBps: number | undefined;
        let etaSeconds: number | undefined;
        if (status === "uploading" && typeof progress.uploadStarted === "number" && uploadedBytes > 0) {
          const elapsedSec = Math.max((Date.now() - progress.uploadStarted) / 1000, 0.001);
          speedBps = uploadedBytes / elapsedSec;
          if (totalBytes && speedBps > 0 && totalBytes > uploadedBytes) {
            etaSeconds = (totalBytes - uploadedBytes) / speedBps;
          }
        }

        const progressPercent =
          typeof progress.percentage === "number"
            ? Math.max(0, Math.min(100, progress.percentage))
            : totalBytes && totalBytes > 0
              ? Math.max(0, Math.min(100, (uploadedBytes / totalBytes) * 100))
              : 0;
        const rawError = file.error as unknown;
        const fileErrorMessage =
          typeof rawError === "string"
            ? rawError
            : rawError && typeof rawError === "object"
              ? String((rawError as { message?: unknown }).message ?? "")
              : undefined;

        const row: UploadRow = {
          id: file.id,
          name: file.name || "unnamed",
          status,
          progressPercent,
          uploadedBytes,
          totalBytes,
          speedBps,
          etaSeconds,
          retries: override?.retries ?? existing?.retries ?? 0,
          message:
            status === "paused" && !isOnline
              ? "Waiting for network..."
              : override?.message ?? fileErrorMessage ?? existing?.message,
          updatedAt: now,
        };

        byId.set(file.id, row);
      }

      for (const [id, existing] of byId.entries()) {
        if (activeIDs.has(id)) continue;
        if (isFinalStatus(existing.status)) continue;
        const override = overridesRef.current[id];
        if (override?.status === "canceled") {
          byId.set(id, {
            ...existing,
            status: "canceled",
            message: override.message ?? "Canceled by user.",
            speedBps: undefined,
            etaSeconds: undefined,
            updatedAt: now,
          });
          continue;
        }
        byId.delete(id);
      }

      return Array.from(byId.values())
        .sort((a, b) => b.updatedAt - a.updatedAt)
        .slice(0, 30);
    });
  }, [isOnline, uppy]);

  useEffect(() => {
    uppy.setMeta({ folderid: folderId ?? "" });
  }, [uppy, folderId]);

  useEffect(() => {
    if (!dashboardRef.current) return;

    uppy.use(Dashboard, {
      inline: true,
      target: dashboardRef.current,
      height: 330,
      proudlyDisplayPoweredByUppy: false,
      showLinkToFileUploadResult: false,
      note: "Resumable uploads via Tus protocol",
    });

    return () => {
      const plugin = uppy.getPlugin("Dashboard");
      if (plugin) {
        uppy.removePlugin(plugin);
      }
    };
  }, [uppy]);

  useEffect(() => {
    const updateNetwork = () => {
      const online = typeof navigator === "undefined" ? true : navigator.onLine;
      setIsOnline(online);
      syncRows();
      if (online) {
        setLastResumeHint("Network restored. Uploads will continue automatically.");
        window.setTimeout(() => setLastResumeHint(null), 4000);
      }
    };

    updateNetwork();
    window.addEventListener("online", updateNetwork);
    window.addEventListener("offline", updateNetwork);
    return () => {
      window.removeEventListener("online", updateNetwork);
      window.removeEventListener("offline", updateNetwork);
    };
  }, [syncRows]);

  useEffect(() => {
    const onUpload = (_uploadID: string, files: any[]) => {
      for (const file of files ?? []) {
        if (file?.id) {
          preparingSetRef.current.add(file.id);
        }
      }
      syncRows();
    };
    const onUploadProgress = (file: any) => {
      preparingSetRef.current.delete(file.id);
      const override = overridesRef.current[file.id];
      if (override?.status === "retrying") {
        delete overridesRef.current[file.id];
      }
      syncRows();
    };
    const onUploadRetry = (fileOrID: any) => {
      const id = typeof fileOrID === "string" ? fileOrID : fileOrID?.id;
      if (!id) return;
      const previous = overridesRef.current[id] ?? {};
      overridesRef.current[id] = {
        ...previous,
        status: "retrying",
        retries: (previous.retries ?? 0) + 1,
        message: "Retrying after a transient upload error...",
      };
      syncRows();
    };
    const onUploadSuccess = (file: any) => {
      preparingSetRef.current.delete(file.id);
      delete overridesRef.current[file.id];
      syncRows();
    };
    const onUploadError = (file: any, error: Error) => {
      preparingSetRef.current.delete(file.id);
      const existing = overridesRef.current[file.id];
      if (existing?.status !== "retrying") {
        overridesRef.current[file.id] = {
          ...existing,
          status: "failed",
          message: error?.message || "Upload failed.",
        };
      }
      syncRows();
    };
    const onFileRemoved = (file: any, reason?: string) => {
      const current = uppy.getFile(file.id);
      if (current) return;
      const text = String(reason ?? "").toLowerCase();
      const canceled = text.includes("removed") || text.includes("cancel");
      if (canceled) {
        overridesRef.current[file.id] = {
          ...(overridesRef.current[file.id] ?? {}),
          status: "canceled",
          message: "Canceled by user.",
        };
      }
      preparingSetRef.current.delete(file.id);
      syncRows();
    };
    const onCancelAll = () => {
      for (const file of uppy.getFiles()) {
        overridesRef.current[file.id] = {
          ...(overridesRef.current[file.id] ?? {}),
          status: "canceled",
          message: "Canceled by user.",
        };
      }
      preparingSetRef.current.clear();
      syncRows();
    };
    const onComplete = () => {
      syncRows();
      onComplete?.();
    };

    uppy.on("file-added", syncRows);
    uppy.on("file-removed", onFileRemoved);
    uppy.on("upload", onUpload);
    uppy.on("upload-progress", onUploadProgress);
    uppy.on("upload-retry", onUploadRetry);
    uppy.on("upload-success", onUploadSuccess);
    uppy.on("upload-error", onUploadError);
    uppy.on("cancel-all", onCancelAll);
    uppy.on("complete", onComplete);

    syncRows();

    return () => {
      uppy.off("file-added", syncRows);
      uppy.off("file-removed", onFileRemoved);
      uppy.off("upload", onUpload);
      uppy.off("upload-progress", onUploadProgress);
      uppy.off("upload-retry", onUploadRetry);
      uppy.off("upload-success", onUploadSuccess);
      uppy.off("upload-error", onUploadError);
      uppy.off("cancel-all", onCancelAll);
      uppy.off("complete", onComplete);
      uppy.destroy();
    };
  }, [onComplete, syncRows, uppy]);

  const activeCount = rows.filter((row) => !isFinalStatus(row.status)).length;

  function sanitizeRelativePathForName(relativePath: string) {
    const segments = relativePath
      .split(/[\\/]+/)
      .map((segment) => segment.trim())
      .filter(Boolean)
      .map((segment) => segment.replace(/[^a-zA-Z0-9._-]+/g, "-"));
    if (segments.length === 0) return "upload-file";
    return segments.join("__");
  }

  function addFolderFilesToUppy(fileList: FileList | null) {
    if (!fileList) return;
    const files = Array.from(fileList);
    if (files.length === 0) return;

    let added = 0;
    let failed = 0;
    for (const file of files) {
      const rawRelative = ((file as unknown as { webkitRelativePath?: string }).webkitRelativePath ?? "").trim();
      const relativePath = rawRelative || file.name;
      const targetName = sanitizeRelativePathForName(relativePath);
      try {
        uppy.addFile({
          name: relativePath,
          type: file.type,
          data: file,
          meta: {
            filename: targetName,
            name: targetName,
            folderid: folderId ?? "",
          },
        });
        added += 1;
      } catch {
        failed += 1;
      }
    }

    if (added > 0 && failed === 0) {
      setFolderUploadHint(`Added ${added} files from selected folder.`);
    } else if (added > 0 && failed > 0) {
      setFolderUploadHint(`Added ${added} files. ${failed} files were skipped (duplicates or invalid names).`);
    } else {
      setFolderUploadHint("No files were added from the selected folder.");
    }

    window.setTimeout(() => setFolderUploadHint(null), 5000);
    if (folderInputRef.current) {
      folderInputRef.current.value = "";
    }
    syncRows();
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => folderInputRef.current?.click()} type="button">
          Add Folder
        </button>
        <input
          className="hidden"
          multiple
          onChange={(event) => addFolderFilesToUppy(event.target.files)}
          ref={folderInputRef}
          type="file"
          {...({ webkitdirectory: "" } as Record<string, string>)}
        />
        <p className="text-xs text-slate-600">
          Folder upload uses browser directory APIs. Files are queued recursively and stored in the current folder.
        </p>
      </div>

      <div ref={dashboardRef} />

      <div className="rounded-xl border border-slate-200 bg-slate-50 p-3">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div>
            <p className="text-sm font-medium">Upload Queue</p>
            <p className="text-xs text-slate-600">
              Tus fingerprint is persisted. After refresh, selecting the same file resumes from the last offset when available.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => uppy.pauseAll()} type="button">
              Pause All
            </button>
            <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => uppy.resumeAll()} type="button">
              Resume All
            </button>
            <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => uppy.cancelAll()} type="button">
              Cancel All
            </button>
          </div>
        </div>

        <div className="mb-2 flex flex-wrap gap-3 text-xs">
          <span className={isOnline ? "text-emerald-700" : "text-amber-700"}>{isOnline ? "Network: online" : "Network: offline"}</span>
          <span className="text-slate-600">Active uploads: {activeCount}</span>
        </div>
        {lastResumeHint ? <p className="mb-2 text-xs text-emerald-700">{lastResumeHint}</p> : null}
        {folderUploadHint ? <p className="mb-2 text-xs text-slate-700">{folderUploadHint}</p> : null}
        <p className="mb-2 text-[11px] text-slate-500">
          Browser limitation: background uploads may pause if the tab is suspended/closed. Uploads can resume later when you return.
        </p>

        {rows.length === 0 ? (
          <p className="text-xs text-slate-600">No upload activity yet.</p>
        ) : (
          <div className="max-h-72 space-y-2 overflow-y-auto pr-1">
            {rows.map((row) => (
              <div className="rounded-lg border border-slate-200 bg-white p-2" key={row.id}>
                <div className="flex items-center justify-between gap-2">
                  <p className="truncate text-xs font-medium">{row.name}</p>
                  <span className="text-[11px] font-medium text-slate-600">{row.status}</span>
                </div>

                <p className="mt-1 text-[11px] text-slate-600">
                  {formatBytes(row.uploadedBytes)}
                  {row.totalBytes ? ` / ${formatBytes(row.totalBytes)}` : ""}
                  {row.totalBytes ? ` (${row.progressPercent.toFixed(1)}%)` : ""}
                </p>

                <p className="text-[11px] text-slate-500">
                  speed: {row.status === "uploading" ? `${formatBytes(row.speedBps)}/s` : "-"} | eta: {formatDuration(row.etaSeconds)} | retries:{" "}
                  {row.retries}
                </p>

                {row.totalBytes ? (
                  <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-200">
                    <div className="h-full bg-pine" style={{ width: `${Math.max(0, Math.min(100, row.progressPercent))}%` }} />
                  </div>
                ) : null}

                {row.message ? <p className="mt-1 text-[11px] text-slate-600">{row.message}</p> : null}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
