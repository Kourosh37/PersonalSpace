import { cpSync, existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { spawn } from "node:child_process";

const root = process.cwd();
const standaloneDir = join(root, ".next", "standalone");
const serverFile = join(standaloneDir, "server.js");

if (!existsSync(serverFile)) {
  console.error("Standalone server not found. Run `npm run build` first.");
  process.exit(1);
}

const staticSource = join(root, ".next", "static");
const staticTarget = join(standaloneDir, ".next", "static");
if (existsSync(staticSource)) {
  mkdirSync(join(standaloneDir, ".next"), { recursive: true });
  cpSync(staticSource, staticTarget, { recursive: true, force: true });
}

const publicSource = join(root, "public");
const publicTarget = join(standaloneDir, "public");
if (existsSync(publicSource)) {
  cpSync(publicSource, publicTarget, { recursive: true, force: true });
}

const child = spawn(process.execPath, [serverFile], {
  cwd: standaloneDir,
  env: {
    ...process.env,
    HOSTNAME: process.env.HOSTNAME ?? "127.0.0.1",
    PORT: process.env.PORT ?? "3000",
  },
  stdio: "inherit",
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});
