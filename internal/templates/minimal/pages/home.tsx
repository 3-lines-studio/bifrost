import "../style.css";

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
    <div className="flex min-h-screen flex-col items-center justify-center gap-2">
      <h1 className="text-3xl font-bold">{message}</h1>
      <p className="text-gray-600">Your Bifrost app is running!</p>
    </div>
  );
}
