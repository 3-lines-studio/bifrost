import React from 'react';
import './style.css';

export function Head({ invoice }: { invoice: { id: string; status: string; amount: number } }) {
  return <title>Invoice {invoice.id}</title>;
}

export function Page({ invoice }: { invoice: { id: string; status: string; amount: number } }) {
  return (
    <main className="p-6">
      <h1>Invoice {invoice.id}</h1>
      <p>Status: {invoice.status}</p>
    </main>
  );
}
