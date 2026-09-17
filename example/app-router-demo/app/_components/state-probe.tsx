import { useState } from "react";
import { usePathname, useSearchParams } from "virtual:bifrost/navigation";

export function StateProbe() {
  const [count, setCount] = useState(0);
  const pathname = usePathname();
  const searchParams = useSearchParams();
  return (
    <div className="note">
      <button type="button" data-probe="state-count" onClick={() => setCount(count + 1)}>local state: {count}</button>
      <p>
        <span className="tag" data-probe="pathname">{pathname}</span>{" "}
        <span className="tag" data-probe="search">{searchParams.toString() || "no query"}</span>
      </p>
      <p>
        <a href={pathname + (searchParams.toString() ? "?" + searchParams.toString() : "")}>same pathname, same query</a>{" · "}
        <a href={pathname + "?tab=one"}>same pathname, new query</a>{" · "}
        <a href="/labs/state/other">new pathname</a>{" · "}
        <a href={pathname + "#anchor"}>hash only</a>
      </p>
    </div>
  );
}
