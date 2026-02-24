import Layout from "@/layout/base";

export function Head() {
  return (
    <>
      <title>Product Page - Static Prerender</title>
      <meta name="description" content="A simple static page prerendered at build time" />
    </>
  );
}

export function Page() {
  return (
    <Layout>
      <div className="max-w-3xl mx-auto p-8">
        <div className="bg-gradient-to-br from-primary to-purple-600 text-primary-foreground p-8 rounded-xl mb-8">
          <h1 className="text-4xl font-bold mb-4">
            Premium Widget
          </h1>
          <p className="text-xl opacity-90">
            The best widget money can buy
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
          {[
            { title: "Feature 1", desc: "Amazing capability A" },
            { title: "Feature 2", desc: "Incredible feature B" },
            { title: "Feature 3", desc: "Outstanding benefit C" },
          ].map((feature, i) => (
            <div
              key={i}
              className="bg-muted p-6 rounded-lg border border-border"
            >
              <h3 className="text-lg font-semibold mb-2 text-foreground">
                {feature.title}
              </h3>
              <p className="text-muted-foreground">{feature.desc}</p>
            </div>
          ))}
        </div>

        <div className="bg-secondary border border-primary p-4 rounded-lg mb-8">
          <strong className="text-primary">Static Prerender Demo:</strong>{" "}
          <span className="text-secondary-foreground">
            This page uses{" "}
            <code className="bg-muted px-1 py-0.5 rounded">WithStatic()</code>{" "}
            with no data loader. The HTML is generated at build time and served 
            as a static file.
          </span>
        </div>

        <a href="/" className="text-primary hover:underline">
          ← Back to Home
        </a>
      </div>
    </Layout>
  );
}
