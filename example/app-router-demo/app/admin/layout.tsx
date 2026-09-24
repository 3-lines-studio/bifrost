import type { ReactNode } from "react";

export const metadata = { robots: { index: false, follow: false } };

export function Layout({ children }: { children: ReactNode }) {
  return (
    <section data-layout="admin">
      <h1>Admin</h1>
      <p className="lede">
        This whole subtree sits behind <code>app/admin/middleware.go</code>, pages and <code>route.go</code> handlers alike.
      </p>
      <nav className="crumbs">
        <a href="/admin">Overview</a>
        <a href="/admin/settings">Settings</a>
        <a href="/admin/api/state">API state</a>
      </nav>
      {children}
    </section>
  );
}
