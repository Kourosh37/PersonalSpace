"use client";

import { useEffect, useMemo, useState } from "react";

type UploadSettings = {
  mode: "unlimited" | "custom";
  maxFileSizeBytes: number | null;
};

const units = ["KB", "MB", "GB", "TB"] as const;
type Unit = (typeof units)[number];

function fromBytes(bytes: number): { value: number; unit: Unit } {
  const map: Record<Unit, number> = {
    KB: 1024,
    MB: 1024 ** 2,
    GB: 1024 ** 3,
    TB: 1024 ** 4,
  };

  for (const unit of ["TB", "GB", "MB", "KB"] as Unit[]) {
    const base = map[unit];
    if (bytes >= base) {
      return { value: Number((bytes / base).toFixed(2)), unit };
    }
  }

  return { value: bytes / 1024, unit: "KB" };
}

function toBytes(value: number, unit: Unit): number {
  const map: Record<Unit, number> = {
    KB: 1024,
    MB: 1024 ** 2,
    GB: 1024 ** 3,
    TB: 1024 ** 4,
  };
  return Math.floor(value * map[unit]);
}

export default function AdminUploadSettingsPage() {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [mode, setMode] = useState<"unlimited" | "custom">("unlimited");
  const [value, setValue] = useState<number>(10);
  const [unit, setUnit] = useState<Unit>("GB");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const response = await fetch("/api/admin/settings/upload", { credentials: "include" });
        if (!response.ok) {
          throw new Error("Failed to load settings");
        }
        const data = (await response.json()) as UploadSettings;
        setMode(data.mode);
        if (data.maxFileSizeBytes && data.maxFileSizeBytes > 0) {
          const parsed = fromBytes(data.maxFileSizeBytes);
          setValue(parsed.value);
          setUnit(parsed.unit);
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to load settings";
        setError(message);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const computedBytes = useMemo(() => {
    if (mode === "unlimited") {
      return null;
    }
    return toBytes(value, unit);
  }, [mode, value, unit]);

  async function save() {
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const payload: UploadSettings = {
        mode,
        maxFileSizeBytes: mode === "custom" ? computedBytes : null,
      };
      const response = await fetch("/api/admin/settings/upload", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(payload),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Failed to save settings");
      }
      setSuccess("Upload settings updated successfully.");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to save settings";
      setError(message);
    } finally {
      setSaving(false);
    }
  }

  function resetDefaults() {
    setMode("unlimited");
    setValue(10);
    setUnit("GB");
    setError(null);
    setSuccess(null);
  }

  if (loading) {
    return <p>Loading upload settings...</p>;
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">Upload Settings</h1>
        <p className="mt-2 text-slate-600">Configure maximum file size policy for uploads.</p>
      </header>

      <section className="panel space-y-5 p-6">
        <div className="space-y-2">
          <label className="flex items-center gap-2">
            <input checked={mode === "unlimited"} onChange={() => setMode("unlimited")} type="radio" />
            <span>Unlimited</span>
          </label>
          <label className="flex items-center gap-2">
            <input checked={mode === "custom"} onChange={() => setMode("custom")} type="radio" />
            <span>Custom size</span>
          </label>
        </div>

        {mode === "custom" ? (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <input
              className="input sm:col-span-2"
              min={1}
              step={1}
              type="number"
              value={value}
              onChange={(event) => setValue(Number(event.target.value || 0))}
            />
            <select className="input" value={unit} onChange={(event) => setUnit(event.target.value as Unit)}>
              {units.map((u) => (
                <option key={u} value={u}>
                  {u}
                </option>
              ))}
            </select>
          </div>
        ) : null}

        <div className="rounded-xl bg-slate-100 px-3 py-2 text-sm text-slate-700">
          Computed bytes: {computedBytes === null ? "null (unlimited)" : computedBytes.toLocaleString()}
        </div>

        {error ? <p className="text-sm text-red-600">{error}</p> : null}
        {success ? <p className="text-sm text-green-700">{success}</p> : null}

        <div className="flex gap-3">
          <button className="btn-primary" disabled={saving} onClick={save} type="button">
            {saving ? "Saving..." : "Save"}
          </button>
          <button className="btn-ghost" onClick={resetDefaults} type="button">
            Reset to default
          </button>
        </div>
      </section>
    </div>
  );
}
