"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";

type MeResponse = {
  user: {
    id: string;
    username: string;
    role: string;
  };
};

type BrowserItem = {
  id: string;
  name: string;
  type: "folder" | "file";
  parentId?: string;
  sizeBytes?: number;
  mimeType?: string;
  extension?: string;
  modifiedAt: string;
  createdAt: string;
};

type ListItemsResponse = {
  parentId?: string;
  items: BrowserItem[];
};

type Crumb = {
  id: string | null;
  name: string;
};

function formatBytes(value?: number) {
  if (!value || value <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = value;
  let unitIdx = 0;
  while (current >= 1024 && unitIdx < units.length - 1) {
    current /= 1024;
    unitIdx += 1;
  }
  return `${current.toFixed(unitIdx === 0 ? 0 : 1)} ${units[unitIdx]}`;
}

export default function DashboardPage() {
  const router = useRouter();
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const [loadingSession, setLoadingSession] = useState(true);
  const [me, setMe] = useState<MeResponse["user"] | null>(null);

  const [loadingItems, setLoadingItems] = useState(false);
  const [itemsError, setItemsError] = useState<string | null>(null);
  const [items, setItems] = useState<BrowserItem[]>([]);

  const [crumbs, setCrumbs] = useState<Crumb[]>([{ id: null, name: "Root" }]);
  const currentParentId = crumbs[crumbs.length - 1]?.id ?? null;

  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState("name");
  const [order, setOrder] = useState("asc");

  const [newFolderName, setNewFolderName] = useState("");
  const [creatingFolder, setCreatingFolder] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [resumableUploading, setResumableUploading] = useState(false);
  const [resumableProgress, setResumableProgress] = useState<Record<string, number>>({});
  const [shareURL, setShareURL] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;
    (async () => {
      try {
        const response = await fetch("/api/auth/me", { credentials: "include" });
        if (!response.ok) {
          router.replace("/login");
          return;
        }
        const data = (await response.json()) as MeResponse;
        if (mounted) setMe(data.user);
      } catch {
        if (mounted) router.replace("/login");
      } finally {
        if (mounted) setLoadingSession(false);
      }
    })();

    return () => {
      mounted = false;
    };
  }, [router]);

  const queryUrl = useMemo(() => {
    const params = new URLSearchParams();
    if (currentParentId) params.set("parentId", currentParentId);
    if (search.trim()) params.set("search", search.trim());
    params.set("sortBy", sortBy);
    params.set("order", order);
    return `/api/folders/items?${params.toString()}`;
  }, [currentParentId, order, search, sortBy]);

  async function reloadItems() {
    setLoadingItems(true);
    setItemsError(null);
    try {
      const response = await fetch(queryUrl, { credentials: "include" });
      if (response.status === 401) {
        router.replace("/login");
        return;
      }
      const data = (await response.json()) as ListItemsResponse & { error?: string };
      if (!response.ok) {
        throw new Error(data.error ?? "Failed to load folder items");
      }
      setItems(data.items ?? []);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load folder items";
      setItemsError(message);
    } finally {
      setLoadingItems(false);
    }
  }

  useEffect(() => {
    if (loadingSession) return;
    void reloadItems();
  }, [loadingSession, queryUrl]);

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST", credentials: "include" });
    router.replace("/login");
  }

  async function changeMyPassword() {
    const currentPassword = window.prompt("Current password:");
    if (!currentPassword) return;
    const newPassword = window.prompt("New password (at least 8 chars):");
    if (!newPassword) return;

    setItemsError(null);
    try {
      const response = await fetch("/api/auth/change-password", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ currentPassword, newPassword }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error ?? "Failed to change password");
      alert("Password changed. Please login again.");
      await logout();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to change password";
      setItemsError(message);
    }
  }

  function openFolder(item: BrowserItem) {
    if (item.type !== "folder") return;
    setCrumbs((prev) => [...prev, { id: item.id, name: item.name }]);
  }

  function goToCrumb(index: number) {
    setCrumbs((prev) => prev.slice(0, index + 1));
  }

  async function createFolder() {
    const name = newFolderName.trim();
    if (!name) return;

    setCreatingFolder(true);
    setItemsError(null);
    try {
      const response = await fetch("/api/folders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ parentId: currentParentId, name }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Failed to create folder");
      }
      setNewFolderName("");
      await reloadItems();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create folder";
      setItemsError(message);
    } finally {
      setCreatingFolder(false);
    }
  }

  async function uploadSelectedFiles() {
    const input = fileInputRef.current;
    const files = input?.files;
    if (!files || files.length === 0) {
      return;
    }

    setUploading(true);
    setItemsError(null);
    setShareURL(null);

    try {
      const form = new FormData();
      Array.from(files).forEach((file) => form.append("file", file));
      const query = new URLSearchParams();
      if (currentParentId) query.set("folderId", currentParentId);

      const response = await fetch(`/api/files/upload?${query.toString()}`, {
        method: "POST",
        body: form,
        credentials: "include",
      });

      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Upload failed");
      }

      await reloadItems();
      if (input) {
        input.value = "";
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Upload failed";
      setItemsError(message);
    } finally {
      setUploading(false);
    }
  }

  async function uploadResumableFiles() {
    const input = fileInputRef.current;
    const files = input?.files;
    if (!files || files.length === 0) return;

    setResumableUploading(true);
    setItemsError(null);
    setShareURL(null);
    setResumableProgress({});

    const chunkSize = 5 * 1024 * 1024;

    try {
      for (const file of Array.from(files)) {
        const initResp = await fetch("/api/uploads/init", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify({
            folderId: currentParentId,
            originalName: file.name,
            targetName: file.name,
            totalSizeBytes: file.size,
          }),
        });
        const initData = await initResp.json().catch(() => ({}));
        if (!initResp.ok) {
          throw new Error(initData?.error ?? `Failed to initialize resumable upload for ${file.name}`);
        }

        const uploadId = initData.id as string;
        let offset = 0;
        while (offset < file.size) {
          const end = Math.min(offset + chunkSize, file.size);
          const chunk = file.slice(offset, end);
          const chunkResp = await fetch(`/api/uploads/${uploadId}/chunk`, {
            method: "PATCH",
            headers: {
              "Content-Type": "application/octet-stream",
              "Upload-Offset": String(offset),
            },
            credentials: "include",
            body: chunk,
          });
          const chunkData = await chunkResp.json().catch(() => ({}));
          if (!chunkResp.ok) {
            throw new Error(chunkData?.error ?? `Chunk upload failed for ${file.name}`);
          }
          offset = Number(chunkData?.uploadedBytes ?? end);
          const percent = Math.min(100, Math.round((offset / file.size) * 100));
          setResumableProgress((prev) => ({ ...prev, [file.name]: percent }));
        }

        const completeResp = await fetch(`/api/uploads/${uploadId}/complete`, {
          method: "POST",
          credentials: "include",
        });
        const completeData = await completeResp.json().catch(() => ({}));
        if (!completeResp.ok) {
          throw new Error(completeData?.error ?? `Failed to finalize upload for ${file.name}`);
        }
        setResumableProgress((prev) => ({ ...prev, [file.name]: 100 }));
      }

      await reloadItems();
      if (input) input.value = "";
    } catch (err) {
      const message = err instanceof Error ? err.message : "Resumable upload failed";
      setItemsError(message);
    } finally {
      setResumableUploading(false);
    }
  }

  async function deleteFolder(item: BrowserItem) {
    if (item.type !== "folder") return;
    const accepted = window.confirm(`Delete folder \"${item.name}\" and all nested items?`);
    if (!accepted) return;

    try {
      const response = await fetch(`/api/folders/${item.id}`, {
        method: "DELETE",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Failed to delete folder");
      }
      await reloadItems();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to delete folder";
      setItemsError(message);
    }
  }

  async function deleteFile(item: BrowserItem) {
    if (item.type !== "file") return;
    const accepted = window.confirm(`Delete file \"${item.name}\"?`);
    if (!accepted) return;

    try {
      const response = await fetch(`/api/files/${item.id}`, {
        method: "DELETE",
        credentials: "include",
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Failed to delete file");
      }
      await reloadItems();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to delete file";
      setItemsError(message);
    }
  }

  async function createShare(item: BrowserItem) {
    setShareURL(null);
    setItemsError(null);
    try {
      const response = await fetch("/api/shares", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          targetType: item.type,
          targetId: item.id,
          allowPreview: true,
          allowDownload: true,
          allowFolderBrowse: true,
        }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data?.error ?? "Failed to create share link");
      }
      setShareURL(data.url ?? null);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create share";
      setItemsError(message);
    }
  }

  if (loadingSession) {
    return <p>Loading session...</p>;
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Dashboard</h1>
          <p className="text-sm text-slate-600">{me ? `Signed in as ${me.username} (${me.role})` : ""}</p>
        </div>
        <div className="flex gap-2">
          <button className="btn-ghost" onClick={changeMyPassword} type="button">
            Change Password
          </button>
          <button className="btn-ghost" onClick={logout} type="button">
            Logout
          </button>
        </div>
      </header>

      <section className="panel p-5">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          {crumbs.map((crumb, index) => (
            <button
              className="rounded-lg px-2 py-1 hover:bg-slate-100"
              key={`${crumb.id ?? "root"}-${index}`}
              onClick={() => goToCrumb(index)}
              type="button"
            >
              {crumb.name}
            </button>
          ))}
        </div>

        <div className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-6">
          <input
            className="input md:col-span-2"
            placeholder="Search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <select className="input" value={sortBy} onChange={(event) => setSortBy(event.target.value)}>
            <option value="name">Sort: Name</option>
            <option value="size">Sort: Size</option>
            <option value="modified">Sort: Modified</option>
            <option value="type">Sort: Type</option>
          </select>
          <select className="input" value={order} onChange={(event) => setOrder(event.target.value)}>
            <option value="asc">Order: Asc</option>
            <option value="desc">Order: Desc</option>
          </select>

          <input
            className="input md:col-span-2"
            placeholder="New folder name"
            value={newFolderName}
            onChange={(event) => setNewFolderName(event.target.value)}
          />
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-3">
          <button className="btn-primary" disabled={creatingFolder} onClick={createFolder} type="button">
            {creatingFolder ? "Creating..." : "Create Folder"}
          </button>

          <input className="input max-w-sm" multiple ref={fileInputRef} type="file" />
          <button className="btn-primary" disabled={uploading} onClick={uploadSelectedFiles} type="button">
            {uploading ? "Uploading..." : "Upload Files"}
          </button>
          <button className="btn-ghost" disabled={resumableUploading} onClick={uploadResumableFiles} type="button">
            {resumableUploading ? "Uploading Resumable..." : "Upload Resumable"}
          </button>
        </div>

        {itemsError ? <p className="mt-3 text-sm text-red-600">{itemsError}</p> : null}
        {Object.keys(resumableProgress).length > 0 ? (
          <div className="mt-3 space-y-1 text-sm text-slate-700">
            {Object.entries(resumableProgress).map(([name, progress]) => (
              <p key={name}>
                {name}: {progress}%
              </p>
            ))}
          </div>
        ) : null}
        {shareURL ? (
          <div className="mt-3 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
            Share URL: <a className="underline" href={shareURL} rel="noreferrer" target="_blank">{shareURL}</a>
          </div>
        ) : null}

        <div className="mt-5 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-slate-600">
                <th className="px-2 py-2">Name</th>
                <th className="px-2 py-2">Type</th>
                <th className="px-2 py-2">Size</th>
                <th className="px-2 py-2">Modified</th>
                <th className="px-2 py-2">Actions</th>
              </tr>
            </thead>
            <tbody>
              {loadingItems ? (
                <tr>
                  <td className="px-2 py-3 text-slate-500" colSpan={5}>
                    Loading items...
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td className="px-2 py-3 text-slate-500" colSpan={5}>
                    Empty folder.
                  </td>
                </tr>
              ) : (
                items.map((item) => (
                  <tr className="border-b border-slate-100" key={item.id}>
                    <td className="px-2 py-2">
                      {item.type === "folder" ? (
                        <button className="text-left text-pine hover:underline" onClick={() => openFolder(item)} type="button">
                          {item.name}
                        </button>
                      ) : (
                        <span>{item.name}</span>
                      )}
                    </td>
                    <td className="px-2 py-2">{item.type}</td>
                    <td className="px-2 py-2">{item.type === "file" ? formatBytes(item.sizeBytes) : "-"}</td>
                    <td className="px-2 py-2">{new Date(item.modifiedAt).toLocaleString()}</td>
                    <td className="px-2 py-2">
                      <div className="flex flex-wrap gap-2">
                        {item.type === "file" ? (
                          <>
                            <a className="btn-ghost !px-2 !py-1 text-xs" href={`/api/files/${item.id}/preview`} rel="noreferrer" target="_blank">
                              Preview
                            </a>
                            <a className="btn-ghost !px-2 !py-1 text-xs" href={`/api/files/${item.id}/download`}>
                              Download
                            </a>
                            <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => deleteFile(item)} type="button">
                              Delete
                            </button>
                          </>
                        ) : (
                          <>
                            <a className="btn-ghost !px-2 !py-1 text-xs" href={`/api/folders/${item.id}/download-zip`}>
                              ZIP
                            </a>
                            <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => deleteFolder(item)} type="button">
                              Delete
                            </button>
                          </>
                        )}
                        <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => createShare(item)} type="button">
                          Share
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
