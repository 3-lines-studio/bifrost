export default function Page(props: { slept: string }) {
  return (
    <>
      <h2>Loading view</h2>
      <p className="note" data-lab="loading">The loader sleeps {props.slept}, so a client navigation shows the nearest loading.tsx first.</p>
      <pre>curl -s localhost:8080/labs/loading | grep data-loading</pre>
    </>
  );
}
