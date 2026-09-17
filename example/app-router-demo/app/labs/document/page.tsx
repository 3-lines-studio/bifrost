export default function Page(props: { lang: string }) {
  return (
    <>
      <h2>Root document attributes</h2>
      <p className="note" data-lab="document">
        The loader returns the lang, class and dir for the root document, so this page renders inside{" "}
        <code>&lt;html lang=&quot;{props.lang}&quot; class=&quot;lab-dark&quot; dir=&quot;ltr&quot;&gt;</code>. Try{" "}
        <a href="/labs/document?lang=en">?lang=en</a> and an invalid one like <a href="/labs/document?lang=not a language">?lang=not a language</a>.
      </p>
      <pre>curl -s localhost:8080/labs/document | head -1</pre>
    </>
  );
}
