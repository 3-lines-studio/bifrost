export default function Page(props: { searchParams: Record<string, string | string[]>; message?: string }) {
  const message = typeof props.searchParams.message === "string" ? props.searchParams.message : "";
  return (
    <>
      <h2>No JavaScript</h2>
      <p className="note" data-lab="nojs">
        Turn JavaScript off and everything here still works: SSR renders the document, links are plain anchors, and this form
        is a normal GET.
      </p>
      <form method="get" action="/labs/nojs">
        <input name="message" defaultValue={message} placeholder="type something" />
        <button type="submit">Send</button>
      </form>
      {message ? <p data-nojs-echo={message}>The server received: {message}</p> : null}
    </>
  );
}
