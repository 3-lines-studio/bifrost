export function Error({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <>
      <h2>Lab error</h2>
      <p className="note bad" data-error="labs">{error.message}</p>
      <button type="button" onClick={() => reset()}>Run it again</button>
    </>
  );
}
