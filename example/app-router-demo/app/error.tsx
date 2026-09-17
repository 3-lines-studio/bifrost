export function Error({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <>
      <h1>Root error</h1>
      <p className="note bad" data-error="root">{error.message}</p>
      <button type="button" onClick={() => reset()}>Try again</button>
    </>
  );
}
