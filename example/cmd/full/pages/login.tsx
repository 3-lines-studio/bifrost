import Layout from "@/layout/base";
import { useState } from "react";

export function Head() {
  return (
    <>
      <title>Login</title>
      <meta name="description" content="Login page for authentication demo" />
    </>
  );
}

export function Page() {
  const [submitted, setSubmitted] = useState(false);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    window.location.href = "/dashboard?demo=true";
  };

  return (
    <Layout>
      <div className="max-w-md mx-auto mt-16 p-8 bg-card rounded-xl shadow-lg">
        <h1 className="text-2xl font-bold mb-6 text-center text-foreground">
          Login
        </h1>

        <div className="bg-secondary border border-primary p-3 rounded-md mb-6 text-sm">
          <strong className="text-primary">Demo:</strong>{" "}
          <span className="text-secondary-foreground">
            Click "Sign In" to simulate authentication and navigate to the 
            protected dashboard.
          </span>
        </div>

        {submitted ? (
          <p className="text-center text-primary">
            Redirecting to dashboard...
          </p>
        ) : (
          <form onSubmit={handleSubmit}>
            <div className="mb-4">
              <label className="block mb-1 text-sm font-medium text-foreground">
                Username
              </label>
              <input
                type="text"
                defaultValue="demo"
                className="w-full p-2 border border-input rounded-md text-base bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
              />
            </div>

            <div className="mb-6">
              <label className="block mb-1 text-sm font-medium text-foreground">
                Password
              </label>
              <input
                type="password"
                defaultValue="password"
                className="w-full p-2 border border-input rounded-md text-base bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
              />
            </div>

            <button
              type="submit"
              className="w-full p-3 bg-primary text-primary-foreground rounded-md text-base font-medium hover:bg-primary/90 transition-colors"
            >
              Sign In
            </button>
          </form>
        )}

        <div className="mt-6 text-center">
          <a href="/" className="text-muted-foreground hover:text-foreground text-sm">
            ← Back to Home
          </a>
        </div>
      </div>
    </Layout>
  );
}
