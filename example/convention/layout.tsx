import type { ReactNode } from "react";

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div>
      <header>Root layout</header>
      {children}
    </div>
  );
}
