export default function Page(props: { searchParams: Record<string, string | string[]> }) {
  if (props.searchParams.boom === "1") {
    throw new Error("the lab threw while rendering");
  }
  return (
    <>
      <h2>Error view</h2>
      <p className="note" data-lab="error">
        Add <code>?boom=1</code> and this page throws. The nearest error.tsx renders instead, and its reset button re-fetches the route.
      </p>
      <p><a href="/labs/error?boom=1">Throw now</a></p>
    </>
  );
}
