import Link from "next/link";

export default function HomePage() {
  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Space</h1>
        <Link className="btn-ghost" href="/login">
          Login
        </Link>
      </header>

      <section className="panel p-6">
        <h2 className="text-xl font-semibold">Private cloud foundation is running</h2>
        <p className="mt-2 text-slate-600">
          Phase 1 includes auth/session core, admin upload size settings, and Dockerized deployment.
        </p>
        <div className="mt-4 flex gap-3">
          <Link className="btn-primary" href="/dashboard">
            Dashboard
          </Link>
          <Link className="btn-ghost" href="/admin/settings/upload">
            Admin Upload Settings
          </Link>
        </div>
      </section>
    </div>
  );
}