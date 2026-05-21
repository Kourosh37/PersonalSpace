"use client";

import { useEffect, useMemo, useState } from "react";

type SettingItem = {
  key: string;
  value: unknown;
  valueType: string;
};

type SharingForm = {
  enabled: boolean;
  publicPreviewEnabled: boolean;
  publicDownloadEnabled: boolean;
  allowFolderSharing: boolean;
  allowPermanentLinks: boolean;
  requirePasswordMode: "optional" | "always" | "disabled";
  defaultExpirationHours: number;
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

function parsePasswordMode(value: unknown): SharingForm["requirePasswordMode"] {
  if (value === "always" || value === "disabled" || value === "optional") return value;
  return "optional";
}

function formFromSettings(items: SettingItem[]): SharingForm {
  const byKey = new Map(items.map((item) => [item.key, item.value]));
  return {
    enabled: parseBool(byKey.get("sharing.enabled"), true),
    publicPreviewEnabled: parseBool(byKey.get("sharing.public_preview_enabled"), true),
    publicDownloadEnabled: parseBool(byKey.get("sharing.public_download_enabled"), true),
    allowFolderSharing: parseBool(byKey.get("sharing.allow_folder_sharing"), true),
    allowPermanentLinks: parseBool(byKey.get("sharing.allow_permanent_links"), true),
    requirePasswordMode: parsePasswordMode(byKey.get("sharing.require_password_mode")),
    defaultExpirationHours: parseNumber(byKey.get("sharing.default_expiration_hours"), 168),
  };
}

export default function SharingSettingsPage() {
  const [form, setForm] = useState<SharingForm>({
    enabled: true,
    publicPreviewEnabled: true,
    publicDownloadEnabled: true,
    allowFolderSharing: true,
    allowPermanentLinks: true,
    requirePasswordMode: "optional",
    defaultExpirationHours: 168,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const response = await fetch("/api/admin/settings/sharing", { credentials: "include" });
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
    return Number.isFinite(form.defaultExpirationHours) && form.defaultExpirationHours >= 0;
  }, [form.defaultExpirationHours]);

  async function save() {
    if (!canSave) {
      setError("Default expiration hours must be zero or greater.");
      return;
    }
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const payload = {
        items: [
          { key: "sharing.enabled", value: form.enabled, valueType: "bool" },
          { key: "sharing.public_preview_enabled", value: form.publicPreviewEnabled, valueType: "bool" },
          { key: "sharing.public_download_enabled", value: form.publicDownloadEnabled, valueType: "bool" },
          { key: "sharing.allow_folder_sharing", value: form.allowFolderSharing, valueType: "bool" },
          { key: "sharing.allow_permanent_links", value: form.allowPermanentLinks, valueType: "bool" },
          { key: "sharing.require_password_mode", value: form.requirePasswordMode, valueType: "string" },
          { key: "sharing.default_expiration_hours", value: Math.floor(form.defaultExpirationHours), valueType: "number" },
        ],
      };
      const response = await fetch("/api/admin/settings/sharing", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(payload),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to save settings");
      setSuccess("Sharing settings updated successfully.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <p>Loading sharing settings...</p>;

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold">Sharing Settings</h1>
      <div className="panel p-5 space-y-4">
        <label className="inline-flex items-center gap-2">
          <input checked={form.enabled} onChange={(e) => setForm((prev) => ({ ...prev, enabled: e.target.checked }))} type="checkbox" />
          Public sharing enabled
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
          <input
            checked={form.publicDownloadEnabled}
            onChange={(e) => setForm((prev) => ({ ...prev, publicDownloadEnabled: e.target.checked }))}
            type="checkbox"
          />
          Public download enabled
        </label>
        <label className="inline-flex items-center gap-2">
          <input
            checked={form.allowFolderSharing}
            onChange={(e) => setForm((prev) => ({ ...prev, allowFolderSharing: e.target.checked }))}
            type="checkbox"
          />
          Allow folder sharing
        </label>
        <label className="inline-flex items-center gap-2">
          <input
            checked={form.allowPermanentLinks}
            onChange={(e) => setForm((prev) => ({ ...prev, allowPermanentLinks: e.target.checked }))}
            type="checkbox"
          />
          Allow permanent links
        </label>

        <div className="space-y-1">
          <label className="text-sm font-medium">Password requirement mode</label>
          <select
            className="input"
            value={form.requirePasswordMode}
            onChange={(e) => setForm((prev) => ({ ...prev, requirePasswordMode: e.target.value as SharingForm["requirePasswordMode"] }))}
          >
            <option value="optional">optional</option>
            <option value="always">always</option>
            <option value="disabled">disabled</option>
          </select>
        </div>

        <div className="space-y-1">
          <label className="text-sm font-medium">Default expiration hours</label>
          <input
            className="input"
            min={0}
            type="number"
            value={form.defaultExpirationHours}
            onChange={(e) => setForm((prev) => ({ ...prev, defaultExpirationHours: Number(e.target.value || 0) }))}
          />
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
