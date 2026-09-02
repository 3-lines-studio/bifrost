import { Suspense } from "react";

let failed = false;
const pending = new Promise<void>((resolve) => {
  setTimeout(() => {
    failed = true;
    resolve();
  }, 200);
});

function Late() {
  if (!failed) {
    throw pending;
  }

  throw new Error("late render failed");
}

export function Page() {
  return (
    <Suspense fallback={<main>Streaming started. The connection will close after the late failure.</main>}>
      <Late />
    </Suspense>
  );
}
