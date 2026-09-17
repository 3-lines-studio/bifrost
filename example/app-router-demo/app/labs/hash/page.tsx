export default function Page(props: { loads: number }) {
  return (
    <>
      <h2>Hash</h2>
      <p className="note" data-lab="hash" data-loads={String(props.loads)}>
        This loader has run {props.loads} times. <a href="#bottom">Jump to the bottom</a> and it does not run again: a
        hash-only change keeps the page and skips the loader.
      </p>
      <p id="bottom">Bottom. Going back restores the scroll position, not the unmounted page state.</p>
    </>
  );
}
