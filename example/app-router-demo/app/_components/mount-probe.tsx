import { useEffect, useState } from "react";

export function MountProbe({ label }: { label: string }) {
  const [mount, setMount] = useState("");
  const [clicks, setClicks] = useState(0);
  useEffect(() => {
    setMount(Math.random().toString(36).slice(2, 7));
  }, []);
  return (
    <p className="note">
      <span data-probe={label} data-mount={mount || "pending"}>
        {label}: {mount ? `mounted ${mount}` : "waiting for the client"}
      </span>
      {" "}
      <button type="button" onClick={() => setClicks(clicks + 1)}>clicks {clicks}</button>
    </p>
  );
}
