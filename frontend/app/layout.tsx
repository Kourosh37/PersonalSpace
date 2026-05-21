import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Space",
  description: "Private file manager",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <main className="mx-auto min-h-screen w-full max-w-6xl px-4 py-8 sm:px-6">{children}</main>
      </body>
    </html>
  );
}