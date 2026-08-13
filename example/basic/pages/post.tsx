import React from "react";

export function Head({ title }: { title: string }) {
  return <title>{title}</title>;
}

export function Page({ title }: { title: string }) {
  return <article><h1>{title}</h1></article>;
}
