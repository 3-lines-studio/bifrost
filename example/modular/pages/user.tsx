import React from 'react';
import './style.css';

export function Head({ user }: { user: { id: string; email: string; plan: string } }) {
  return <title>{user.email}</title>;
}

export function Page({ user }: { user: { id: string; email: string; plan: string } }) {
  return (
    <main className="p-6">
      <h1>{user.email}</h1>
      <p>Plan: {user.plan}</p>
    </main>
  );
}
