import React, { Suspense, lazy, useState } from "react";
import logo from "./logo.svg";
import styles from "./button.module.css";
import copy from "@example/copy.json";
import { source } from "virtual:bifrost-example";
import { href, routes } from "virtual:bifrost/routes";

const LazyClientFeature = lazy(() => import("./lazy"));

export function Page() {
  const [count, setCount] = useState(0);
  return <main><img src={logo} width="16" height="16" alt="" /><button className={styles.button} data-copy={copy.label} data-plugin={source} onClick={() => setCount(count + 1)}>Count {count}</button><nav><a href={href("/post/{slug}", { slug: "first" })} data-routes={routes.length}>First post</a></nav><Suspense fallback={<span>Loading</span>}><LazyClientFeature /></Suspense></main>;
}
