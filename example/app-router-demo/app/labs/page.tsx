const labs = [
  ["/labs/state", "Client state", "resets on pathname change, survives query and hash changes"],
  ["/labs/loading", "Loading view", "a slow loader plus the nearest loading.tsx"],
  ["/labs/error", "Error view", "a page that throws and a button that resets it"],
  ["/labs/loader-error", "Loader error", "a loader that returns bifrost.Status(503, err)"],
  ["/labs/notfound", "Not found", "a loader that returns bifrost.NotFound()"],
  ["/labs/query?tag=one&tag=two", "Query", "repeated keys become arrays, and a GET form works without JS"],
  ["/labs/middleware", "Middleware", "three levels composing in order"],
  ["/labs/metadata", "Metadata", "layout metadata merged with a page override and generateMetadata"],
  ["/labs/document", "Root document", "lang, class and dir per request, from the loader"],
  ["/labs/request", "Request scope", "one loader call per request, no shared state"],
  ["/labs/nojs", "No JavaScript", "SSR and a plain form"],
  ["/labs/reload", "Reload and downloads", "data-bifrost-reload, external links, downloads"],
  ["/labs/hash", "Hash", "hash-only navigation does not run the loader"],
];

export default function Page() {
  return (
    <ul className="cards">
      {labs.map(([href, title, hint]) => (
        <li key={href}><a href={href}>{title}<small>{hint}</small></a></li>
      ))}
    </ul>
  );
}
