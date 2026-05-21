"use client";

import { useEffect, useState } from "react";

type Setting = {
  key: string;
  value: unknown;
  valueType: string;
};

export default function SharingSettingsPage() {
  const [items, setItems] = useState<Setting[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const response = await fetch("/api/admin/settings/sharing", { credentials: "include" });
        const data = await response.json();
        if (!response.ok) throw new Error(data?.error ?? "Failed to load settings");
        setItems(data.items ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load settings");
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  async function save() {
    setSaving(true);
    setError(null);
    try {
      const payload = {
        items: items.map((i) => ({ key: i.key, value: i.value, valueType: i.valueType || "json" })),
      };
      const response = await fetch("/api/admin/settings/sharing", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(payload),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data?.error ?? "Failed to save settings");
      setItems(data.items ?? items);
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
      {error ? <p className="text-sm text-red-600">{error}</p> : null}
      <div className="panel p-5 space-y-3">
        {items.map((item, index) => (
          <div className="space-y-1" key={item.key}>
            <label className="text-sm font-medium">{item.key}</label>
            <input
              className="input"
              value={typeof item.value === "string" ? item.value : JSON.stringify(item.value)}
              onChange={(event) => {
                const next = [...items];
                next[index] = { ...item, value: event.target.value };
                setItems(next);
              }}
            />
          </div>
        ))}
        <button className="btn-primary" type="button" onClick={save} disabled={saving}>
          {saving ? "Saving..." : "Save"}
        </button>
      </div>
    </div>
  );
}