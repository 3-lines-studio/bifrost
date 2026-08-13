import React from "react";
import logo from "./logo.svg";

export function Head({ name }: { name?: string }) {
  return <title>{`Hello ${name || "World"}`}</title>;
}

export function Page({ name }: { name?: string }) {
  return <main><img src={logo} width="16" height="16" alt="" /><h1>Hello {name || "World"}</h1></main>;
}
