import "../style.css";

export function Head() {
	return (
		<>
			<title>SPA Example</title>
			<meta name="description" content="Bifrost SPA" />
		</>
	);
}

export function Page() {
	return (
		<div className="flex min-h-screen flex-col items-center justify-center gap-2">
			<h1 className="text-3xl font-bold">Single Page Application</h1>
			<p className="text-gray-600">This is a client-only SPA template!</p>
		</div>
	);
}
