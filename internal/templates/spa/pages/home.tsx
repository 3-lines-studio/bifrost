export function Head() {
	return (
		<>
			<title>SPA Example</title>
			<meta name="description" content="Bifrost SPA" />
		</>
	);
}

export default function Home() {
	return (
		<div
			style={{
				padding: "2rem",
				fontFamily: "system-ui, sans-serif",
			}}
		>
			<h1>Single Page Application</h1>
			<p>This is a client-only SPA template!</p>
		</div>
	);
}
