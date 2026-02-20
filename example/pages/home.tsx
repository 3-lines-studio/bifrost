import { Button } from "@/components/ui/button";
import { Hello } from "@/components/hello";
import Layout from "@/layout/base";
import { themeScript } from "@/lib/theme-script";

export function Head({ name }: { name: string }) {
  return (
    <>
      <title>{`Hello, ${name}`}</title>
      <meta name="description" content={`Hello ${name} from bifrost`} />
      <script dangerouslySetInnerHTML={{ __html: themeScript }} />
    </>
  );
}

export function Page({ name }: { name: string }) {
  const pageLinks = [
    { path: "/", label: "Home" },
    { path: "/about", label: "About" },
    { path: "/nested", label: "Nested" },
    { path: "/blog/hello-world", label: "Blog: Hello World" },
    { path: "/blog/getting-started", label: "Blog: Getting Started" },
    { path: "/message/123", label: "Dynamic Message" },
    { path: "/error", label: "Error Handler Demo" },
    { path: "/error-render", label: "Render Error Demo" },
    { path: "/error-import", label: "Import Error Demo" },
  ];

  return (
    <Layout>
      <Hello name={name} />
      <Button onClick={() => console.log("hello button")}>Hola mundo</Button>

      <div style={{ marginTop: "2rem" }}>
        <h2 style={{ fontSize: "1.25rem", marginBottom: "1rem" }}>
          Test Pages
        </h2>
        <ul style={{ listStyle: "none", padding: 0 }}>
          {pageLinks.map((link) => (
            <li key={link.path} style={{ marginBottom: "0.5rem" }}>
              <a
                href={link.path}
                style={{ color: "#3b82f6", textDecoration: "underline" }}
              >
                {link.label}
              </a>
              <span
                style={{
                  color: "#6b7280",
                  marginLeft: "0.5rem",
                  fontSize: "0.875rem",
                }}
              >
                {link.path}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </Layout>
  );
}
