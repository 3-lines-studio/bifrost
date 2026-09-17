type Project = { Slug: string; Name: string; Summary: string; Log: { Author: string; Text: string }[] };

export default function Page(props: { projects: Project[] }) {
  return (
    <>
      <ul className="cards">
        {props.projects.map((project) => (
          <li key={project.Slug}>
            <a href={`/projects/${project.Slug}`}>
              {project.Name}<small>{project.Summary}</small>
            </a>
          </li>
        ))}
        <li><a href="/projects/new">A brand new project<small>a static folder wins over the dynamic one</small></a></li>
        <li><a href="/projects/nope">A project that does not exist<small>the loader returns 404</small></a></li>
      </ul>
    </>
  );
}
