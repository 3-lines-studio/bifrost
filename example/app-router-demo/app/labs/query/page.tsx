export const metadata = { title: "Query lab" };

export default function Page(props: { pathname: string; tags: string[]; searchParams: Record<string, string | string[]> }) {
  const tags = props.tags ?? [];
  return (
    <>
      <h2>Query</h2>
      <p className="note" data-lab="query">
        <code>?tag=one&amp;tag=two</code> arrives as an array: {JSON.stringify(tags)}. The same page also receives{" "}
        <code>pathname</code> = {props.pathname}.
      </p>
      <form method="get" action="/labs/query">
        <input name="tag" placeholder="tag" />
        <input name="tag" placeholder="another tag" />
        <button type="submit">Filter</button>
      </form>
      <p><small>The form is a plain GET, so it works with JavaScript off.</small></p>
    </>
  );
}
