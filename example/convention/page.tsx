export function Head() {
  return <title>Bifrost convention app</title>;
}

export function Page() {
  return (
    <main>
      <h1>Bifrost convention app</h1>
      <p>Custom Vite config: {__CONVENTION_EXAMPLE__ ? "active" : "inactive"}</p>
      <ul>
        <li><a href="/posts/hello">Loader, dynamic params, middleware, and layouts</a></li>
        <li><a href="/posts/api/hello">All-method REST route</a></li>
        <li><a href="/posts/missing/path">Nearest not-found page</a></li>
        <li><a href="/missing">Root not-found page</a></li>
        <li><a href="/posts/render-error">Synchronous render error</a></li>
        <li><a href="/broken">Failing nested error boundary with root fallback</a></li>
        <li><a href="/late">Late streaming error</a></li>
        <li><a href="/robots.txt">Public file</a></li>
        <li><a href="/healthz">Custom server health route</a></li>
      </ul>
      <pre>curl -X PATCH http://localhost:8080/posts/api/hello</pre>
    </main>
  );
}
