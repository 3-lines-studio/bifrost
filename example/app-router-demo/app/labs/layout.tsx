import type { ReactNode } from "react";
import { MountProbe } from "../_components/mount-probe";

export function Layout({ children }: { children: ReactNode }) {
  return (
    <section data-layout="labs">
      <h1>Labs</h1>
      <p className="lede">One experiment per behaviour. Each page says what it proves and how to check it without a browser.</p>
      <MountProbe label="labs layout" />
      <nav className="crumbs" data-labs-nav="">
        <a href="/labs">index</a>
        <a href="/labs/state">state</a>
        <a href="/labs/loading">loading</a>
        <a href="/labs/error">error</a>
        <a href="/labs/loader-error">loader error</a>
        <a href="/labs/notfound">not found</a>
        <a href="/labs/query?tag=one&tag=two">query</a>
        <a href="/labs/middleware">middleware</a>
        <a href="/labs/metadata">metadata</a>
        <a href="/labs/document">document</a>
        <a href="/labs/request">request scope</a>
        <a href="/labs/nojs">no js</a>
        <a href="/labs/reload">reload</a>
        <a href="/labs/hash">hash</a>
      </nav>
      {children}
    </section>
  );
}
