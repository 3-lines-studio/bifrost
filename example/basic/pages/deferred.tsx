import React from "react";

await new Promise((resolve) => setTimeout(resolve, 25));

export default function DeferredContent() {
  return <strong>Stream complete</strong>;
}
