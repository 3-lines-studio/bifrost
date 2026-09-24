export function Error({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <>
      <h2>Docs error</h2>
      <p className="note bad" data-error="docs">{error.message}</p>
      <button type="button" onClick={() => reset()}>Reload the docs</button>
    </>
  );
}
