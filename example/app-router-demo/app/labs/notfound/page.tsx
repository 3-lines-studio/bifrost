export default function Page(props: { ok: boolean }) {
  return (
    <p className="note" data-lab="notfound">
      The loader returned a value, so the page rendered. Remove <code>?ok=1</code> and the loader returns bifrost.NotFound,
      which renders the labs section not-found.tsx.
    </p>
  );
}
