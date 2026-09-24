import { StateProbe } from "../../_components/state-probe";

export default function Page() {
  return (
    <>
      <h2>Client state</h2>
      <p>
        Click the counter, then follow each link. The counter resets when the pathname changes, survives a query or hash
        change, and the loader does not run on a hash-only change.
      </p>
      <StateProbe />
    </>
  );
}
