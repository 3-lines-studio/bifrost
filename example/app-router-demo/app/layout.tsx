import type { ReactNode } from "react";
import { Link, usePathname } from "virtual:bifrost/navigation";

export const metadata = {
  title: "Bifrost App Router demo",
  description: "An app that exercises every App Router feature on purpose.",
  robots: { index: false, follow: false },
};

const sections = [
  { href: "/", label: "Home" },
  { href: "/docs", label: "Docs" },
  { href: "/projects", label: "Projects" },
  { href: "/labs", label: "Labs" },
  { href: "/admin", label: "Admin" },
];

export function Layout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return (
    <>
      <link rel="stylesheet" href="/styles.css" precedence="high" />
      <header className="shell">
        <a className="brand" href="/">Bifrost App Router demo</a>
        <nav className="crumbs" data-nav={pathname}>
          {sections.map((section) => (
            <Link key={section.href} href={section.href} aria-current={pathname === section.href ? "page" : undefined}>
              {section.label}
            </Link>
          ))}
          <a href="/api/search?q=marker">API</a>
          <a href="/docs/markers.md">Markdown</a>
        </nav>
      </header>
      <main className="shell">{children}</main>
      <footer className="shell">
        <p>Every page here is a feature of the App Router, and <code>make check</code> asserts it.</p>
      </footer>
    </>
  );
}
