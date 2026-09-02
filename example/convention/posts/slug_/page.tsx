export function Page({ slug, middleware }: { slug: string; middleware: string[] }) {
  if (slug === "render-error") {
    throw new Error("render failed");
  }

  return (
    <main>
      <a href="/">Home</a>
      <h1>Post: {slug}</h1>
      <p>Middleware: {middleware.join(" → ")}</p>
      <ul>
        <li><a href={`?result=redirect`}>Redirect</a></li>
        <li><a href={`?result=not-found`}>Not found</a></li>
        <li><a href={`?result=forbidden`}>Forbidden</a></li>
        <li><a href={`?result=error`}>Loader error</a></li>
        <li><a href="/posts/render-error">Render error</a></li>
      </ul>
    </main>
  );
}
