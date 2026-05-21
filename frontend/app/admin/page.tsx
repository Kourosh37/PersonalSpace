import Link from "next/link";

export default function AdminHomePage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Admin</h1>
      <div className="grid gap-3 sm:grid-cols-2">
        <Link className="panel p-5 hover:border-pine" href="/admin/settings">
          <h2 className="text-lg font-semibold">Settings</h2>
          <p className="mt-1 text-slate-600">Upload, sharing, preview and public settings.</p>
        </Link>
        <Link className="panel p-5 hover:border-pine" href="/admin/storage">
          <h2 className="text-lg font-semibold">Storage</h2>
          <p className="mt-1 text-slate-600">Usage summary, recalculate, cleanup expired uploads.</p>
        </Link>
        <Link className="panel p-5 hover:border-pine" href="/admin/logs">
          <h2 className="text-lg font-semibold">Audit Logs</h2>
          <p className="mt-1 text-slate-600">Security and operational event history.</p>
        </Link>
      </div>
    </div>
  );
}
