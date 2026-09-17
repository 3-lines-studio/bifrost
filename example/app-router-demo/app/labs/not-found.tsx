export function NotFound() {
  return (
    <>
      <h2>No such lab</h2>
      <p className="note warn" data-not-found="labs">
        The labs section has its own not-found.tsx, so a lab that calls bifrost.NotFound gets this view instead of the root one.
      </p>
    </>
  );
}
