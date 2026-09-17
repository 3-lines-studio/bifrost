export default function Page() {
  return (
    <>
      <h2>Middleware</h2>
      <p className="note" data-lab="middleware">
        This page is wrapped by <code>app/middleware.go</code> and <code>app/labs/middleware.go</code>. Check the order in the
        loader props and in the response headers.
      </p>
      <pre>curl -sI localhost:8080/labs/middleware | grep -i x-demo</pre>
    </>
  );
}
