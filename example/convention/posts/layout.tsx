import type { ReactNode } from "react";

export function Layout({ children }: { children: ReactNode }) {
  return <section><p>Posts layout</p>{children}</section>;
}
