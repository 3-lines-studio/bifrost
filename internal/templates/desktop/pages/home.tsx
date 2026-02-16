export function Head() {
	return (
		<>
			<title>Desktop App</title>
			<meta name="description" content="Bifrost Desktop Application" />
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
			<h1>Bifrost Desktop</h1>
			<p>Your desktop app is running!</p>
		</div>
	);
}
