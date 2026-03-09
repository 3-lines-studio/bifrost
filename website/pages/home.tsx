import Layout from "../layout/base";

interface NavItem {
  slug: string;
  title: string;
}

interface HomeProps {
  nav: NavItem[];
}

export function Head() {
  return (
    <>
      <title>Bifrost — SSR for Go</title>
      <meta
        name="description"
        content="Server-side rendering for React and Svelte components in Go. Bridge your backend with modern frontends."
      />
    </>
  );
}

export function Page({ nav }: HomeProps) {
  const features = [
    {
      title: "Multi-Framework",
      desc: "React and Svelte with identical Go APIs.",
    },
    {
      title: "Server-Side Rendering",
      desc: "Full SSR with data loading from Go handlers.",
    },
    {
      title: "Static Generation",
      desc: "Prerender pages at build time. No runtime needed.",
    },
    {
      title: "Single Binary",
      desc: "Embed everything into one Go binary for deployment.",
    },
    {
      title: "Hot Reload",
      desc: "Instant feedback during development.",
    },
    {
      title: "Bun-Powered",
      desc: "Fast builds and SSR powered by Bun.",
    },
  ];

  const codeExample = `package main

import (
    "embed"
    "log"
    "net/http"

    "github.com/3-lines-studio/bifrost"
)

//go:embed all:.bifrost
var bifrostFS embed.FS

func main() {
    app := bifrost.New(
        bifrostFS,
        bifrost.Page("/", "./pages/home.tsx",
            bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
                return map[string]any{"name": "World"}, nil
            }),
        ),
    )
    defer app.Stop()

    log.Fatal(http.ListenAndServe(":8080", app.Handler()))
}`;

  return (
    <Layout nav={nav}>
      <section className="max-w-4xl mx-auto px-6 pt-24 pb-16">
        <div className="max-w-2xl">
          <h1 className="text-4xl sm:text-5xl font-bold tracking-tight leading-[1.1] mb-6">
            Server-side rendering
            <br />
            for <span className="text-accent">Go</span>
          </h1>
          <p className="text-lg text-muted leading-relaxed mb-10 max-w-lg">
            Bridge your Go backend with React and Svelte frontends. SSR, static
            generation, and single-binary deployment.
          </p>

          <div className="flex flex-wrap gap-3">
            <a
              href="/docs/getting-started"
              className="inline-flex items-center px-5 py-2.5 bg-fg text-bg rounded-lg text-sm font-medium hover:opacity-90 transition-opacity"
            >
              Get Started
            </a>
            <div className="flex items-center gap-2 px-4 py-2.5 bg-code-bg border border-border rounded-lg text-sm font-mono text-muted">
              <span>go get github.com/3-lines-studio/bifrost</span>
            </div>
          </div>
        </div>
      </section>

      <section className="max-w-4xl mx-auto px-6 pb-24">
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-6">
          {features.map((feature) => (
            <div
              key={feature.title}
              className="p-5 rounded-xl border border-border bg-surface"
            >
              <h3 className="font-semibold mb-1.5 text-fg">{feature.title}</h3>
              <p className="text-sm text-muted leading-relaxed">
                {feature.desc}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section className="border-t border-border">
        <div className="max-w-4xl mx-auto px-6 py-16">
          <h2 className="text-2xl font-semibold tracking-tight mb-8">
            Quick Start
          </h2>
          <div className="bg-code-bg border border-border rounded-xl p-6 overflow-x-auto">
            <pre className="text-sm leading-relaxed">
              <code>{codeExample}</code>
            </pre>
          </div>
        </div>
      </section>
    </Layout>
  );
}
