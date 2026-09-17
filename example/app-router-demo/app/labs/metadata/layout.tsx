import type { ReactNode } from "react";

export const metadata = {
  title: "Metadata lab",
  description: "Set by the lab layout.",
  openGraph: { title: "Metadata lab", description: "Set by the lab layout." },
};

export function Layout({ children }: { children: ReactNode }) {
  return <section data-layout="metadata">{children}</section>;
}
