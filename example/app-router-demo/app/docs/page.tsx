export default function Page(props: { paths: string[]; loads: number }) {
  return (
    <>
      <h2>Index</h2>
      <p className="lede">This loader has run {props.loads} times.</p>
      <ul className="cards">
        {props.paths.map((path) => (
          <li key={path || "index"}>
            <a href={path ? `/docs/${path}` : "/docs"}>{path || "docs index"}<small>catch-all route</small></a>
          </li>
        ))}
        <li><a href="/docs/does-not-exist">a missing doc<small>bifrost.NotFound from the loader</small></a></li>
      </ul>
    </>
  );
}
