export const metadata = {
  title: "The page overrides the layout title",
  keywords: ["app router", "metadata"],
};

export default function Page() {
  return (
    <>
      <h2>Metadata</h2>
      <p className="note" data-lab="metadata">
        The layout sets a title, description and openGraph; this page overrides the title and adds keywords. Look at the head
        of the HTML, not at the body. The docs section shows the per-request half with generateMetadata.
      </p>
      <pre>curl -s localhost:8080/labs/metadata?from=curl | grep -o '&lt;meta[^&gt;]*&gt;' | head</pre>
    </>
  );
}
