export const metadata = { title: "Demo login" };

export default function Page(props: { searchParams: Record<string, string | string[]> }) {
  const next = typeof props.searchParams.next === "string" ? props.searchParams.next : "/admin";
  return (
    <>
      <h2>Sign in</h2>
      <form method="post" action={`/admin/login?next=${encodeURIComponent(next)}`}>
        <button type="submit">Enter as demo admin</button>
      </form>
      <p><small>No JavaScript needed: the form posts to <code>route.go</code>, which sets the cookie and redirects.</small></p>
    </>
  );
}
