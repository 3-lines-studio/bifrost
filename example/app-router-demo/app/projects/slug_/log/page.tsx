type Project = { Slug: string; Name: string; Log: { Author: string; Text: string }[] };

export default function Page(props: { project: Project; params: { slug: string } }) {
  return (
    <section data-log={props.params.slug}>
      <h3>{props.project.Name} log</h3>
      {props.project.Log.length === 0 ? (
        <p className="note">Empty. Post one through <code>route.go</code> and the page refreshes.</p>
      ) : (
        <table className="data">
          <thead><tr><th>author</th><th>entry</th></tr></thead>
          <tbody>
            {props.project.Log.map((entry, index) => (
              <tr key={index}><td>{entry.Author}</td><td>{entry.Text}</td></tr>
            ))}
          </tbody>
        </table>
      )}
      <form method="post" action={`/projects/${props.params.slug}`}>
        <input name="text" placeholder="new log entry" />
        <button type="submit">Append</button>
      </form>
      <p><small>The form posts to <code>route.go</code> with a redirect back here, so it works with JavaScript off.</small></p>
    </section>
  );
}
