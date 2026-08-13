import React, { lazy, Suspense } from "react";

const Deferred = lazy(() => import("./deferred"));

export function Head() {
  return <title>Stream</title>;
}

export function Page() {
  return <main><Suspense fallback={<span>Loading</span>}><Deferred /></Suspense></main>;
}
