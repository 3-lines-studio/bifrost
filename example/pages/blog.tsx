import Layout from "@/layout/base";
import { themeScript } from "@/lib/theme-script";

export function Head({ title }: { title: string }) {
  return (
    <>
      <title>{title}</title>
      <meta name="description" content={`Blog post: ${title}`} />
      <script dangerouslySetInnerHTML={{ __html: themeScript }} />
    </>
  );
}

export function Page({ title, body }: { title: string; body: string }) {
  return (
    <Layout>
      <article>
        <h1 style={{ fontSize: "2rem", marginBottom: "1rem" }}>{title}</h1>
        <div style={{ lineHeight: "1.6" }}>{body}</div>
        <div style={{ marginTop: "2rem" }}>
          <a href="/" style={{ color: "#3b82f6", textDecoration: "underline" }}>
            ← Back to Home
          </a>
        </div>
      </article>
    </Layout>
  );
}
