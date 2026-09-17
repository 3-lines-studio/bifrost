import type { ReactNode } from "react";

export const metadata = { keywords: ["bifrost", "app router", "demo"] };

export function Layout({ children }: { children: ReactNode }) {
  return (
    <section data-layout="projects">
      <h1>Projects</h1>
      <p className="lede">A dynamic slug, a nested log page, and mutations that go through route.go.</p>
      {children}
    </section>
  );
}
