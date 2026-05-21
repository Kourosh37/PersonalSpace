"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

type MeResponse = {
  user: {
    id: string;
    username: string;
    role: string;
  };
};

export default function DashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [me, setMe] = useState<MeResponse["user"] | null>(null);

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
        if (mounted) {
          setMe(data.user);
        }
      } catch {
        if (mounted) {
          setError("Failed to load session");
        }
      } finally {
        if (mounted) {
          setLoading(false);
        }
      }
    })();

    return () => {
      mounted = false;
    };
  }, [router]);

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST", credentials: "include" });
    router.replace("/login");
  }

  if (loading) {
    return <p>Loading dashboard...</p>;
  }

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Dashboard</h1>
        <button className="btn-ghost" onClick={logout} type="button">
          Logout
        </button>
      </header>

      <section className="panel p-6">
        <h2 className="text-lg font-semibold">Welcome{me ? `, ${me.username}` : ""}</h2>
        <p className="mt-2 text-slate-600">
          File browser/upload/share engine is being implemented in next phase.
        </p>
        {error ? <p className="mt-3 text-sm text-red-600">{error}</p> : null}
      </section>
    </div>
  );
}