import Layout from "@/layout/base";
import { Suspense } from "react";

function StreamChild() {
  return <span data-testid="stream-child">streamed</span>;
}

export function Head() {
  return <title>Stream SSR demo</title>;
}

export function Page() {
  return (
    <Layout>
      <main data-testid="stream-demo-root">
        <h1>Stream SSR demo</h1>
        <Suspense fallback={<p data-testid="stream-fallback">loading</p>}>
          <StreamChild />
        </Suspense>
      </main>
    </Layout>
  );
}
