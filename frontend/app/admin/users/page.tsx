"use client";

import { useEffect, useState } from "react";

type User = {
  id: string;
  username: string;
  role: "admin" | "user";
  isActive: boolean;
  storageQuotaBytes?: number;
  usedStorageBytes: number;
  createdAt: string;
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

export default function AdminUsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const [saving, setSaving] = useState(false);

  async function load() {
    setError(null);
    try {
      const response = await fetch("/api/admin/users", { credentials: "include" });
      const data = await response.json();
      if (!response.ok) throw new Error(data?.error ?? "Failed to load users");
      setUsers(data.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function createUser() {
    setSaving(true);
    setError(null);
    try {
      const response = await fetch("/api/admin/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ username, password, role }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to create user");
      setUsername("");
      setPassword("");
      setRole("user");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create user");
    } finally {
      setSaving(false);
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
      await load();
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
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
          <input className="input" placeholder="Username" value={username} onChange={(e) => setUsername(e.target.value)} />
          <input className="input" placeholder="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          <select className="input" value={role} onChange={(e) => setRole(e.target.value as "admin" | "user") }>
            <option value="user">user</option>
            <option value="admin">admin</option>
          </select>
          <button className="btn-primary" type="button" onClick={createUser} disabled={saving}>
            {saving ? "Creating..." : "Create"}
          </button>
        </div>
      </section>

      <section className="panel overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-200 text-slate-600">
              <th className="px-2 py-2">Username</th>
              <th className="px-2 py-2">Role</th>
              <th className="px-2 py-2">Status</th>
              <th className="px-2 py-2">Used</th>
              <th className="px-2 py-2">Quota</th>
              <th className="px-2 py-2">Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map((user) => (
              <tr key={user.id} className="border-b border-slate-100">
                <td className="px-2 py-2">{user.username}</td>
                <td className="px-2 py-2">{user.role}</td>
                <td className="px-2 py-2">{user.isActive ? "active" : "disabled"}</td>
                <td className="px-2 py-2">{formatBytes(user.usedStorageBytes)}</td>
                <td className="px-2 py-2">{formatBytes(user.storageQuotaBytes)}</td>
                <td className="px-2 py-2">
                  <div className="flex gap-2">
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
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}