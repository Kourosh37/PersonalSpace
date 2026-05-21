"use client";

import { useEffect, useMemo, useState } from "react";

type SettingItem = {
  key: string;
  value: unknown;
  valueType: string;
};

type PreviewForm = {
  enabled: boolean;
  publicPreviewEnabled: boolean;
  officeEnabled: boolean;
  mediaEnabled: boolean;
  imageThumbnailsEnabled: boolean;
  textMaxBytes: number;
  csvMaxRows: number;
  officeTimeoutSeconds: number;
  autoPreviewMaxBytes: number;
};

function parseBool(value: unknown, fallback: boolean) {
  if (typeof value === "boolean") return value;
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    if (normalized === "true") return true;
    if (normalized === "false") return false;
  }
  return fallback;
}

function parseNumber(value: unknown, fallback: number) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function formFromSettings(items: SettingItem[]): PreviewForm {
  const byKey = new Map(items.map((item) => [item.key, item.value]));
  return {
    enabled: parseBool(byKey.get("preview.enabled"), true),
    publicPreviewEnabled: parseBool(byKey.get("preview.public_preview_enabled"), true),
    officeEnabled: parseBool(byKey.get("preview.office_enabled"), false),
    mediaEnabled: parseBool(byKey.get("preview.media_enabled"), true),
    imageThumbnailsEnabled: parseBool(byKey.get("preview.image_thumbnails_enabled"), true),
    textMaxBytes: parseNumber(byKey.get("preview.text_max_bytes"), 1024 * 1024),
    csvMaxRows: parseNumber(byKey.get("preview.csv_max_rows"), 500),
    officeTimeoutSeconds: parseNumber(byKey.get("preview.office_conversion_timeout_seconds"), 120),
    autoPreviewMaxBytes: parseNumber(byKey.get("preview.max_auto_generate_size_bytes"), 100 * 1024 * 1024),
  };
}

function formatBytes(value: number) {
  const units = ["B", "KB", "MB", "GB"];
  let current = value;
  let idx = 0;
  while (current >= 1024 && idx < units.length - 1) {
    current /= 1024;
    idx += 1;
  }
  return `${current.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`;
}

export default function PreviewSettingsPage() {
  const [form, setForm] = useState<PreviewForm>({
    enabled: true,
    publicPreviewEnabled: true,
    officeEnabled: false,
    mediaEnabled: true,
    imageThumbnailsEnabled: true,
    textMaxBytes: 1024 * 1024,
    csvMaxRows: 500,
    officeTimeoutSeconds: 120,
    autoPreviewMaxBytes: 100 * 1024 * 1024,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const response = await fetch("/api/admin/settings/preview", { credentials: "include" });
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data?.error ?? "Failed to load settings");
        setForm(formFromSettings((data.items ?? []) as SettingItem[]));
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load settings");
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const canSave = useMemo(() => {
    return (
      Number.isFinite(form.textMaxBytes) &&
      Number.isFinite(form.csvMaxRows) &&
      Number.isFinite(form.officeTimeoutSeconds) &&
      Number.isFinite(form.autoPreviewMaxBytes) &&
      form.textMaxBytes > 0 &&
      form.csvMaxRows > 0 &&
      form.officeTimeoutSeconds > 0 &&
      form.autoPreviewMaxBytes > 0
    );
  }, [form]);

  async function save() {
    if (!canSave) {
      setError("Numeric values must be positive.");
      return;
    }
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const payload = {
        items: [
          { key: "preview.enabled", value: form.enabled, valueType: "bool" },
          { key: "preview.public_preview_enabled", value: form.publicPreviewEnabled, valueType: "bool" },
          { key: "preview.office_enabled", value: form.officeEnabled, valueType: "bool" },
          { key: "preview.media_enabled", value: form.mediaEnabled, valueType: "bool" },
          { key: "preview.image_thumbnails_enabled", value: form.imageThumbnailsEnabled, valueType: "bool" },
          { key: "preview.text_max_bytes", value: Math.floor(form.textMaxBytes), valueType: "number" },
          { key: "preview.csv_max_rows", value: Math.floor(form.csvMaxRows), valueType: "number" },
          { key: "preview.office_conversion_timeout_seconds", value: Math.floor(form.officeTimeoutSeconds), valueType: "number" },
          { key: "preview.max_auto_generate_size_bytes", value: Math.floor(form.autoPreviewMaxBytes), valueType: "number" },
        ],
      };
      const response = await fetch("/api/admin/settings/preview", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(payload),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to save settings");
      setSuccess("Preview settings updated successfully.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <p>Loading preview settings...</p>;

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold">Preview Settings</h1>
      <div className="panel p-5 space-y-4">
        <label className="inline-flex items-center gap-2">
          <input checked={form.enabled} onChange={(e) => setForm((prev) => ({ ...prev, enabled: e.target.checked }))} type="checkbox" />
          Preview enabled
        </label>
        <label className="inline-flex items-center gap-2">
          <input
            checked={form.publicPreviewEnabled}
            onChange={(e) => setForm((prev) => ({ ...prev, publicPreviewEnabled: e.target.checked }))}
            type="checkbox"
          />
          Public preview enabled
        </label>
        <label className="inline-flex items-center gap-2">
          <input checked={form.officeEnabled} onChange={(e) => setForm((prev) => ({ ...prev, officeEnabled: e.target.checked }))} type="checkbox" />
          Office preview enabled
        </label>
        <label className="inline-flex items-center gap-2">
          <input checked={form.mediaEnabled} onChange={(e) => setForm((prev) => ({ ...prev, mediaEnabled: e.target.checked }))} type="checkbox" />
          Media preview enabled
        </label>
        <label className="inline-flex items-center gap-2">
          <input
            checked={form.imageThumbnailsEnabled}
            onChange={(e) => setForm((prev) => ({ ...prev, imageThumbnailsEnabled: e.target.checked }))}
            type="checkbox"
          />
          Image thumbnails enabled
        </label>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <label className="text-sm font-medium">Text preview max bytes</label>
            <input
              className="input"
              min={1}
              type="number"
              value={form.textMaxBytes}
              onChange={(e) => setForm((prev) => ({ ...prev, textMaxBytes: Number(e.target.value || 0) }))}
            />
            <p className="text-xs text-slate-500">{formatBytes(form.textMaxBytes)}</p>
          </div>

          <div className="space-y-1">
            <label className="text-sm font-medium">CSV max rows</label>
            <input
              className="input"
              min={1}
              type="number"
              value={form.csvMaxRows}
              onChange={(e) => setForm((prev) => ({ ...prev, csvMaxRows: Number(e.target.value || 0) }))}
            />
          </div>

          <div className="space-y-1">
            <label className="text-sm font-medium">Office conversion timeout (seconds)</label>
            <input
              className="input"
              min={1}
              type="number"
              value={form.officeTimeoutSeconds}
              onChange={(e) => setForm((prev) => ({ ...prev, officeTimeoutSeconds: Number(e.target.value || 0) }))}
            />
          </div>

          <div className="space-y-1">
            <label className="text-sm font-medium">Auto preview max file size (bytes)</label>
            <input
              className="input"
              min={1}
              type="number"
              value={form.autoPreviewMaxBytes}
              onChange={(e) => setForm((prev) => ({ ...prev, autoPreviewMaxBytes: Number(e.target.value || 0) }))}
            />
            <p className="text-xs text-slate-500">{formatBytes(form.autoPreviewMaxBytes)}</p>
          </div>
        </div>

        {error ? <p className="text-sm text-red-600">{error}</p> : null}
        {success ? <p className="text-sm text-green-700">{success}</p> : null}

        <button className="btn-primary" type="button" onClick={save} disabled={saving || !canSave}>
          {saving ? "Saving..." : "Save"}
        </button>
      </div>
    </div>
  );
}
