import type { ReactNode } from "react";

const pages = ["", "markers", "loaders", "errors", "middleware", "metadata"];

export function Layout({ children }: { children: ReactNode }) {
  return (
    <section data-layout="docs">
      <h1>Docs</h1>
      <p className="lede">This whole section is nested inside the root layout, with its own loading, error and not-found views.</p>
      <nav className="crumbs" data-docs-nav="">
        {pages.map((page) => (
          <a key={page || "index"} href={page ? `/docs/${page}` : "/docs"}>{page || "index"}</a>
        ))}
      </nav>
      {children}
    </section>
  );
}
