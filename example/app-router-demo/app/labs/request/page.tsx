export default function Page(props: { id: string; loads: number; path: string }) {
  return (
    <>
      <h2>Request scope</h2>
      <p className="note" data-lab="request" data-request={props.id}>
        Each request runs its own loader: this one says {props.id} and it is call number {props.loads}. Fire ten requests at
        once and every id stays unique.
      </p>
      <pre>for i in $(seq 10); do curl -s localhost:8080/labs/request &amp;; done | grep -o 'data-request="[a-f0-9]*"' | sort -u | wc -l</pre>
    </>
  );
}
