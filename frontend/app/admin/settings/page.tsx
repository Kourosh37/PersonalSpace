"use client";

import Link from "next/link";

export default function AdminSettingsPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Admin Settings</h1>
      <div className="grid gap-3 sm:grid-cols-2">
        <Link className="panel p-4 hover:border-pine" href="/admin/settings/upload">
          <h2 className="font-semibold">Upload Settings</h2>
          <p className="mt-1 text-sm text-slate-600">Max file size and upload behavior.</p>
        </Link>
        <Link className="panel p-4 hover:border-pine" href="/admin/settings/sharing">
          <h2 className="font-semibold">Sharing Settings</h2>
          <p className="mt-1 text-sm text-slate-600">Public sharing defaults and limits.</p>
        </Link>
        <Link className="panel p-4 hover:border-pine" href="/admin/settings/preview">
          <h2 className="font-semibold">Preview Settings</h2>
          <p className="mt-1 text-sm text-slate-600">Preview toggles and thresholds.</p>
        </Link>
      </div>
    </div>
  );
}