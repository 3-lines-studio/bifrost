const links = [
  { href: "/docs", title: "Docs", hint: "catch-all route, nested layout, own loading and error" },
  { href: "/projects", title: "Projects", hint: "dynamic slug, nested log page, mutations through route.go" },
  { href: "/labs", title: "Labs", hint: "one experiment per App Router behaviour" },
  { href: "/admin", title: "Admin", hint: "middleware that guards a whole subtree" },
  { href: "/api/search?q=marker", title: "JSON API", hint: "route.go next to pages, same middleware" },
  { href: "/docs/markers.md", title: "Markdown", hint: "the same route, served as Markdown" },
];

export default function Page(props: { projects: number; docs: number; loads: number }) {
  return (
    <>
      <h1>Everything the App Router does, in one app</h1>
      <p className="lede">
        {props.projects} projects, {props.docs} docs, and this page has been loaded {props.loads} times.
      </p>
      <ul className="cards">
        {links.map((link) => (
          <li key={link.href}>
            <a href={link.href}>{link.title}<small>{link.hint}</small></a>
          </li>
        ))}
      </ul>
      <div className="note">
        Nothing here is a mock: the same tree runs in production and development, and <code>scripts/check.sh</code> verifies each claim with curl.
      </div>
    </>
  );
}
