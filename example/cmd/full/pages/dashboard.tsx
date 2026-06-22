import Layout from "@/layout/base";

export function Head() {
  return (
    <>
      <title>Dashboard - Protected</title>
      <meta name="description" content="Protected dashboard page" />
    </>
  );
}

export function Page({ user }: { user: { name: string; role: string } }) {
  return (
    <Layout>
      <div className="max-w-5xl mx-auto p-8">
        <div className="bg-gradient-to-br from-emerald-500 to-emerald-600 text-white p-6 rounded-xl mb-8">
          <div className="flex justify-between items-center">
            <div>
              <h1 className="text-2xl font-bold mb-1">
                Dashboard
              </h1>
              <p className="opacity-90">
                Welcome back, {user.name}
              </p>
            </div>
            <div className="bg-white/20 px-4 py-2 rounded-full text-sm">
              {user.role}
            </div>
          </div>
        </div>

        <div className="bg-muted border border-primary p-4 rounded-lg mb-8">
          <strong className="text-primary">RedirectError Demo:</strong>{" "}
          <span className="text-muted-foreground">
            If you try to access this page without being authenticated, the 
            server returns a redirect to /login. This demonstrates the 
            RedirectError interface.
          </span>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
          {[
            { label: "Total Users", value: "1,234" },
            { label: "Revenue", value: "$12,345" },
            { label: "Orders", value: "567" },
            { label: "Growth", value: "+23%" },
          ].map((stat, i) => (
            <div
              key={i}
              className="bg-card p-6 rounded-lg border border-border text-center"
            >
              <div className="text-sm text-muted-foreground mb-2">
                {stat.label}
              </div>
              <div className="text-2xl font-semibold text-foreground">
                {stat.value}
              </div>
            </div>
          ))}
        </div>

        <div className="flex gap-4">
          <a href="/" className="text-primary hover:underline">
            ← Back to Home
          </a>
          <span className="text-border">|</span>
          <a href="/login" className="text-muted-foreground hover:text-foreground">
            Logout
          </a>
        </div>
      </div>
    </Layout>
  );
}
