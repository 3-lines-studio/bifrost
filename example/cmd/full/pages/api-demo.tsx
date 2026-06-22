import Layout from "@/layout/base";
import { useState, useEffect } from "react";

export function Head() {
  return (
    <>
      <title>API Demo - Async Data Loading</title>
      <meta name="description" content="Demonstrates async data loading in SSR" />
    </>
  );
}

interface User {
  id: number;
  name: string;
  email: string;
}

export function Page({ users, loadTime }: { users: User[]; loadTime: string }) {
  const [clientUsers, setClientUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setTimeout(() => {
      setClientUsers([
        { id: 4, name: "Client Alice", email: "alice@example.com" },
        { id: 5, name: "Client Bob", email: "bob@example.com" },
      ]);
      setLoading(false);
    }, 500);
  }, []);

  return (
    <Layout>
      <div className="max-w-3xl mx-auto p-8">
        <h1 className="text-3xl font-bold mb-6 text-foreground">
          Async Data Loading Demo
        </h1>
        
        <div className="bg-muted p-4 rounded-lg mb-8">
          <h2 className="text-xl font-semibold mb-2 text-foreground">
            Server-Side Loaded Data
          </h2>
          <p className="text-muted-foreground mb-4">
            Loaded at: {loadTime}
          </p>
          <ul className="space-y-2">
            {users.map((user) => (
              <li 
                key={user.id}
                className="p-3 bg-card rounded border border-border"
              >
                <span className="font-semibold text-foreground">{user.name}</span>
                <span className="text-muted-foreground"> - {user.email}</span>
              </li>
            ))}
          </ul>
        </div>

        <div className="bg-muted p-4 rounded-lg">
          <h2 className="text-xl font-semibold mb-2 text-foreground">
            Client-Side Loaded Data
          </h2>
          <p className="text-muted-foreground mb-4">
            Loaded after hydration with 500ms delay
          </p>
          {loading ? (
            <p className="text-muted-foreground">Loading client data...</p>
          ) : (
            <ul className="space-y-2">
              {clientUsers.map((user) => (
                <li 
                  key={user.id}
                  className="p-3 bg-card rounded border border-border"
                >
                  <span className="font-semibold text-foreground">{user.name}</span>
                  <span className="text-muted-foreground"> - {user.email}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="mt-8">
          <a href="/" className="text-primary hover:underline">
            ← Back to Home
          </a>
        </div>
      </div>
    </Layout>
  );
}
