import Link from "next/link";

export default function AdminHomePage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Admin</h1>
      <section className="panel p-6">
        <h2 className="text-lg font-semibold">Settings</h2>
        <p className="mt-2 text-slate-600">Manage global application behavior.</p>
        <div className="mt-4">
          <Link className="btn-primary" href="/admin/settings/upload">
            Upload Settings
          </Link>
        </div>
      </section>
    </div>
  );
}