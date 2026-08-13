import React from "react";
import "../style.css";
import { WorkspaceBadge } from "@bifrost/example-ui";
import { greeting } from "@structured/message";

export function Head({ name }: { name: string }) {
  return <title>{name}</title>;
}

export function Page({ name }: { name: string }) {
  return <main className="p-4"><h1>{greeting} {name}</h1><WorkspaceBadge /></main>;
}
