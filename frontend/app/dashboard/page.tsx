"use client";

import { useEffect, useMemo, useState } from "react";
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

  useEffect(() => {
    if (loadingSession) return;

    let mounted = true;
    (async () => {
      setLoadingItems(true);
      setItemsError(null);
      try {
        const response = await fetch(queryUrl, { credentials: "include" });
        if (response.status === 401) {
          router.replace("/login");
          return;
        }
        const data = (await response.json()) as ListItemsResponse;
        if (!response.ok) {
          throw new Error((data as unknown as { error?: string }).error ?? "Failed to load folder items");
        }
        if (mounted) setItems(data.items ?? []);
      } catch (err) {
        const message = err instanceof Error ? err.message : "Failed to load folder items";
        if (mounted) setItemsError(message);
      } finally {
        if (mounted) setLoadingItems(false);
      }
    })();

    return () => {
      mounted = false;
    };
  }, [loadingSession, queryUrl, router]);

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST", credentials: "include" });
    router.replace("/login");
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
      const listResponse = await fetch(queryUrl, { credentials: "include" });
      const listData = (await listResponse.json()) as ListItemsResponse;
      if (listResponse.ok) {
        setItems(listData.items ?? []);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to create folder";
      setItemsError(message);
    } finally {
      setCreatingFolder(false);
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
      setItems((prev) => prev.filter((candidate) => candidate.id !== item.id));
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to delete folder";
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
        <button className="btn-ghost" onClick={logout} type="button">
          Logout
        </button>
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

        <div className="mt-3">
          <button className="btn-primary" disabled={creatingFolder} onClick={createFolder} type="button">
            {creatingFolder ? "Creating..." : "Create Folder"}
          </button>
        </div>

        {itemsError ? <p className="mt-3 text-sm text-red-600">{itemsError}</p> : null}

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
                      {item.type === "folder" ? (
                        <button className="btn-ghost !px-2 !py-1 text-xs" onClick={() => deleteFolder(item)} type="button">
                          Delete
                        </button>
                      ) : (
                        <span className="text-slate-400">-</span>
                      )}
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