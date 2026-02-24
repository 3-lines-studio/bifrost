export function Head({ message }: { message: string }) {
  return (
    <>
      <title>Welcome to Bifrost</title>
      <meta name="description" content="Bifrost app" />
    </>
  );
}

export function Page({ message }: { message: string }) {
  return (
    <div style={{ padding: "2rem", fontFamily: "system-ui, sans-serif" }}>
      <h1>{message}</h1>
      <p>Your Bifrost app is running!</p>
    </div>
  );
}
