export function Error({ error, reset }: { error: Error; reset: () => void }) {
  return <main><h1>Posts error</h1><p>{error.message}</p><button onClick={reset}>Retry</button></main>;
}
