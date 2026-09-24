export function generateMetadata(props: { title?: string; params: { slug: string[] } }) {
  return {
    title: (props.title ?? "Docs") + " · docs",
    description: "Documentation for " + props.params.slug.join("/") + ".",
  };
}

export default function Page(props: { title: string; body: string; docs: string[]; middleware: string[]; loads: number; params: { slug: string[] } }) {
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
