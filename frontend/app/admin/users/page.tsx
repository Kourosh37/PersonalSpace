"use client";

import { useEffect, useMemo, useState } from "react";

type User = {
  id: string;
  username: string;
  role: "admin" | "user";
  isActive: boolean;
  storageQuotaBytes?: number;
  usedStorageBytes: number;
  createdAt: string;
};

type UserDraft = {
  username: string;
  role: "admin" | "user";
  isActive: boolean;
  quotaGB: string;
};

function formatBytes(value?: number) {
  if (!value || value <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let index = 0;
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024;
    index += 1;
  }
  return `${current.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function bytesToGBString(value?: number) {
  if (!value || value <= 0) return "";
  return (value / (1024 ** 3)).toFixed(2).replace(/\.00$/, "");
}

function gbStringToBytes(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed <= 0) return NaN;
  return Math.floor(parsed * (1024 ** 3));
}

function makeDraft(user: User): UserDraft {
  return {
    username: user.username,
    role: user.role,
    isActive: user.isActive,
    quotaGB: bytesToGBString(user.storageQuotaBytes),
  };
}

export default function AdminUsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [drafts, setDrafts] = useState<Record<string, UserDraft>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [savingId, setSavingId] = useState<string | null>(null);

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const [isActive, setIsActive] = useState(true);
  const [quotaGB, setQuotaGB] = useState("");
  const [creating, setCreating] = useState(false);

  async function load() {
    setError(null);
    try {
      const response = await fetch("/api/admin/users", { credentials: "include" });
      const data = await response.json();
      if (!response.ok) throw new Error(data?.error ?? "Failed to load users");
      const items = (data.items ?? []) as User[];
      setUsers(items);
      const nextDrafts: Record<string, UserDraft> = {};
      for (const user of items) {
        nextDrafts[user.id] = makeDraft(user);
      }
      setDrafts(nextDrafts);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const sortedUsers = useMemo(() => {
    return [...users].sort((a, b) => a.username.localeCompare(b.username));
  }, [users]);

  async function createUser() {
    setCreating(true);
    setError(null);
    try {
      const quotaBytes = gbStringToBytes(quotaGB);
      if (Number.isNaN(quotaBytes)) {
        throw new Error("Quota must be a positive GB value");
      }

      const response = await fetch("/api/admin/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          username: username.trim(),
          password,
          role,
          isActive,
          storageQuotaBytes: quotaBytes,
        }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to create user");
      setUsername("");
      setPassword("");
      setRole("user");
      setIsActive(true);
      setQuotaGB("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create user");
    } finally {
      setCreating(false);
    }
  }

  function updateDraft(userID: string, patch: Partial<UserDraft>) {
    setDrafts((prev) => ({
      ...prev,
      [userID]: { ...prev[userID], ...patch },
    }));
  }

  async function saveUser(user: User) {
    const draft = drafts[user.id];
    if (!draft) return;

    setSavingId(user.id);
    setError(null);
    try {
      const quotaBytes = gbStringToBytes(draft.quotaGB);
      if (Number.isNaN(quotaBytes)) {
        throw new Error("Quota must be a positive GB value");
      }

      const body: Record<string, unknown> = {
        username: draft.username.trim(),
        role: draft.role,
        isActive: draft.isActive,
      };
      if (quotaBytes === null) {
        body.clearStorageQuota = true;
      } else {
        body.storageQuotaBytes = quotaBytes;
      }

      const response = await fetch(`/api/admin/users/${user.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(body),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to update user");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update user");
    } finally {
      setSavingId(null);
    }
  }

  async function deactivateUser(user: User) {
    if (!window.confirm(`Deactivate ${user.username}?`)) return;
    setError(null);
    try {
      const response = await fetch(`/api/admin/users/${user.id}`, {
        method: "DELETE",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to deactivate user");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to deactivate user");
    }
  }

  async function resetPassword(user: User) {
    const next = window.prompt(`New password for ${user.username}:`);
    if (!next) return;
    setError(null);
    try {
      const response = await fetch(`/api/admin/users/${user.id}/change-password`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ newPassword: next }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to change password");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to change password");
    }
  }

  if (loading) return <p>Loading users...</p>;

  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-semibold">Users</h1>

      {error ? <p className="text-sm text-red-600">{error}</p> : null}

      <section className="panel p-5 space-y-3">
        <h2 className="text-lg font-semibold">Create User</h2>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-6">
          <input className="input sm:col-span-2" placeholder="Username" value={username} onChange={(e) => setUsername(e.target.value)} />
          <input className="input sm:col-span-2" placeholder="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          <select className="input" value={role} onChange={(e) => setRole(e.target.value as "admin" | "user")}>
            <option value="user">user</option>
            <option value="admin">admin</option>
          </select>
          <input
            className="input"
            placeholder="Quota (GB, blank=unlimited)"
            value={quotaGB}
            onChange={(e) => setQuotaGB(e.target.value)}
          />
        </div>
        <label className="inline-flex items-center gap-2 text-sm">
          <input checked={isActive} onChange={(e) => setIsActive(e.target.checked)} type="checkbox" />
          Active
        </label>
        <button className="btn-primary" type="button" onClick={createUser} disabled={creating}>
          {creating ? "Creating..." : "Create"}
        </button>
      </section>

      <section className="panel overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 text-slate-600">
              <th className="px-2 py-2">Username</th>
              <th className="px-2 py-2">Role</th>
              <th className="px-2 py-2">Status</th>
              <th className="px-2 py-2">Used</th>
              <th className="px-2 py-2">Quota (GB)</th>
              <th className="px-2 py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {sortedUsers.map((user) => {
              const draft = drafts[user.id] ?? makeDraft(user);
              const busy = savingId === user.id;
              return (
                <tr key={user.id} className="border-b border-slate-100 align-top">
                  <td className="px-2 py-2">
                    <input
                      className="input"
                      value={draft.username}
                      onChange={(e) => updateDraft(user.id, { username: e.target.value })}
                    />
                  </td>
                  <td className="px-2 py-2">
                    <select className="input" value={draft.role} onChange={(e) => updateDraft(user.id, { role: e.target.value as "admin" | "user" })}>
                      <option value="user">user</option>
                      <option value="admin">admin</option>
                    </select>
                  </td>
                  <td className="px-2 py-2">
                    <label className="inline-flex items-center gap-2">
                      <input
                        checked={draft.isActive}
                        onChange={(e) => updateDraft(user.id, { isActive: e.target.checked })}
                        type="checkbox"
                      />
                      {draft.isActive ? "active" : "disabled"}
                    </label>
                  </td>
                  <td className="px-2 py-2 whitespace-nowrap">
                    <div>{formatBytes(user.usedStorageBytes)}</div>
                    <div className="text-xs text-slate-500">{user.createdAt ? new Date(user.createdAt).toLocaleDateString() : ""}</div>
                  </td>
                  <td className="px-2 py-2">
                    <input
                      className="input"
                      placeholder="blank = unlimited"
                      value={draft.quotaGB}
                      onChange={(e) => updateDraft(user.id, { quotaGB: e.target.value })}
                    />
                    <p className="mt-1 text-xs text-slate-500">Current: {formatBytes(user.storageQuotaBytes)}</p>
                  </td>
                  <td className="px-2 py-2">
                    <div className="flex flex-wrap gap-2">
                      <button className="btn-primary !px-2 !py-1 text-xs" type="button" onClick={() => void saveUser(user)} disabled={busy}>
                        {busy ? "Saving..." : "Save"}
                      </button>
                      <button className="btn-ghost !px-2 !py-1 text-xs" type="button" onClick={() => resetPassword(user)}>
                        Change Password
                      </button>
                      {user.isActive ? (
                        <button className="btn-ghost !px-2 !py-1 text-xs" type="button" onClick={() => deactivateUser(user)}>
                          Deactivate
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>
    </div>
  );
}
