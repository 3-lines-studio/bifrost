type Project = { Slug: string; Name: string; Summary: string; Log: { Author: string; Text: string }[] };

export default function Page(props: { project: Project; middleware: string[]; params: { slug: string } }) {
  return (
    <article data-project={props.params.slug}>
      <h2>{props.project.Name}</h2>
      <p className="lede">{props.project.Summary}</p>
      <p>
        <span className="tag">params.slug = {props.params.slug}</span>{" "}
        <span className="tag">middleware: {props.middleware.join(" → ")}</span>
      </p>
      <p><a href={`/projects/${props.params.slug}/log`}>Open the log</a> · <a href={`/projects/${props.params.slug}/log.md`}>as Markdown</a></p>
    </article>
  );
}
