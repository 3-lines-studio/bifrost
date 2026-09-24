export default function Page() {
  return (
    <>
      <h2>Static beats dynamic</h2>
      <p className="note">
        This page lives in <code>app/projects/new</code>, and the dynamic route is <code>app/projects/slug_</code>.
        The static folder wins, exactly like in Next.
      </p>
    </>
  );
}
