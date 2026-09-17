export function generateMetadata(props: { title?: string; missing?: string; params: { slug: string[] } }) {
  if (props.missing) {
    return { title: "No doc · docs" };
  }
  return {
    title: (props.title ?? "Docs") + " · docs",
    description: "Documentation for " + props.params.slug.join("/") + ".",
  };
}

export default function Page(props: { title: string; body: string; docs: string[]; middleware: string[]; loads: number; missing?: string; params: { slug: string[] } }) {
  if (props.missing) {
    return (
      <article data-missing={props.missing}>
        <h2>No doc at /{props.params.slug.join("/")}</h2>
        <p className="note warn">
          The catch-all loader answered 404 itself. See the README: the same page with <code>bifrost.NotFound()</code> and no
          not-found view of its own returns a 500 today.
        </p>
      </article>
    );
  }
  return (
    <article data-doc={props.params.slug.join("/")}>
      <h2>{props.title}</h2>
      <p className="lede">{props.body}</p>
      <table className="data">
        <tbody>
          <tr><th>catch-all segments</th><td data-segments="">{JSON.stringify(props.params.slug)}</td></tr>
          <tr><th>middleware order</th><td>{props.middleware.join(" → ")}</td></tr>
          <tr><th>this loader ran</th><td>{props.loads} times</td></tr>
        </tbody>
      </table>
    </article>
  );
}
