"use client";

import { useEffect, useMemo, useState } from "react";
import { useParams } from "next/navigation";

type ShareInfo = {
  id: string;
  targetType: "file" | "folder";
  targetId: string;
  name: string;
  allowPreview: boolean;
  allowDownload: boolean;
  allowFolderBrowse: boolean;
  expiresAt?: string;
  passwordRequired: boolean;
};

type Item = {
  id: string;
  name: string;
  type: "file" | "folder";
  parentId?: string;
  sizeBytes?: number;
  modifiedAt: string;
};

function bytes(value?: number) {
  if (!value) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let i = 0;
  while (current >= 1024 && i < units.length - 1) {
    current /= 1024;
    i++;
  }
  return `${current.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export default function PublicSharePage() {
  const params = useParams<{ token: string }>();
  const token = params.token;

  const [password, setPassword] = useState("");
  const [authorizedPassword, setAuthorizedPassword] = useState<string>("");
  const [share, setShare] = useState<ShareInfo | null>(null);
  const [items, setItems] = useState<Item[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [parentId, setParentId] = useState<string | null>(null);

  const baseQuery = useMemo(() => {
    const q = new URLSearchParams();
    if (authorizedPassword) q.set("password", authorizedPassword);
    if (parentId) q.set("parentId", parentId);
    return q.toString();
  }, [authorizedPassword, parentId]);

  useEffect(() => {
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(`/api/public/shares/${token}${authorizedPassword ? `?password=${encodeURIComponent(authorizedPassword)}` : ""}`);
        const data = await response.json().catch(() => ({}));
        if (!response.ok) {
          throw new Error(data?.error ?? "Failed to load share");
        }

        const info = data as ShareInfo;
        setShare(info);

        if (info.targetType === "folder" && info.allowFolderBrowse) {
          const itemsResponse = await fetch(`/api/public/shares/${token}/items${baseQuery ? `?${baseQuery}` : ""}`);
          const itemsData = await itemsResponse.json().catch(() => ({}));
          if (!itemsResponse.ok) {
            throw new Error(itemsData?.error ?? "Failed to load shared items");
          }
          setItems(itemsData.items ?? []);
        } else {
          setItems([]);
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to load share";
        setError(message);
      } finally {
        setLoading(false);
      }
    })();
  }, [token, authorizedPassword, baseQuery]);

  async function verifyPassword() {
    setError(null);
    try {
      const response = await fetch(`/api/public/shares/${token}/password`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Password check failed");
      }
      setAuthorizedPassword(password);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Password check failed";
      setError(message);
    }
  }

  if (loading) return <p>Loading share...</p>;

  if (error) {
    return (
      <div className="panel p-6">
        <h1 className="text-xl font-semibold">Share</h1>
        <p className="mt-2 text-red-600">{error}</p>
      </div>
    );
  }

  if (!share) return null;

  if (share.passwordRequired && !authorizedPassword) {
    return (
      <div className="mx-auto max-w-md panel p-6 space-y-4">
        <h1 className="text-xl font-semibold">Protected Share</h1>
        <p className="text-sm text-slate-600">This share requires a password.</p>
        <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        <button className="btn-primary w-full" type="button" onClick={verifyPassword}>
          Unlock
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <header className="panel p-5">
        <h1 className="text-2xl font-semibold">Shared: {share.name}</h1>
        <p className="mt-1 text-sm text-slate-600">Type: {share.targetType}</p>
      </header>

      {share.targetType === "file" ? (
        <section className="panel p-5 flex gap-3">
          {share.allowPreview ? (
            <a className="btn-ghost" href={`/api/public/shares/${token}/files/${share.targetId}/preview${authorizedPassword ? `?password=${encodeURIComponent(authorizedPassword)}` : ""}`} target="_blank" rel="noreferrer">
              Preview
            </a>
          ) : null}
          {share.allowDownload ? (
            <a className="btn-primary" href={`/api/public/shares/${token}/files/${share.targetId}/download${authorizedPassword ? `?password=${encodeURIComponent(authorizedPassword)}` : ""}`}>
              Download
            </a>
          ) : null}
        </section>
      ) : (
        <section className="panel p-5 space-y-3">
          <div className="flex gap-2">
            <button className="btn-ghost" type="button" onClick={() => setParentId(null)}>
              Root
            </button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 text-slate-600">
                  <th className="px-2 py-2">Name</th>
                  <th className="px-2 py-2">Type</th>
                  <th className="px-2 py-2">Size</th>
                  <th className="px-2 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr className="border-b border-slate-100" key={item.id}>
                    <td className="px-2 py-2">
                      {item.type === "folder" ? (
                        <button className="text-pine hover:underline" onClick={() => setParentId(item.id)} type="button">
                          {item.name}
                        </button>
                      ) : (
                        item.name
                      )}
                    </td>
                    <td className="px-2 py-2">{item.type}</td>
                    <td className="px-2 py-2">{item.type === "file" ? bytes(item.sizeBytes) : "-"}</td>
                    <td className="px-2 py-2">
                      {item.type === "file" ? (
                        <div className="flex gap-2">
                          {share.allowPreview ? (
                            <a className="btn-ghost !px-2 !py-1 text-xs" href={`/api/public/shares/${token}/files/${item.id}/preview${authorizedPassword ? `?password=${encodeURIComponent(authorizedPassword)}` : ""}`} target="_blank" rel="noreferrer">
                              Preview
                            </a>
                          ) : null}
                          {share.allowDownload ? (
                            <a className="btn-primary !px-2 !py-1 text-xs" href={`/api/public/shares/${token}/files/${item.id}/download${authorizedPassword ? `?password=${encodeURIComponent(authorizedPassword)}` : ""}`}>
                              Download
                            </a>
                          ) : null}
                        </div>
                      ) : (
                        ""
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}