<script lang="ts">
	import Layout from '../layout/base.svelte';
	import Hello from '../components/hello.svelte';
	
	let { name }: { name: string } = $props();
	
	const sections = [
		{
			title: "Core Pages",
			description: "Basic navigation pages",
			links: [
				{ path: "/", label: "Home", mode: "SSR", desc: "This page" },
				{ path: "/about", label: "About", mode: "Client-Only", desc: "About page" },
				{ path: "/login", label: "Login", mode: "Client-Only", desc: "Authentication demo" },
			]
		},
		{
			title: "SSR Examples",
			description: "Server-side rendering with various data loading patterns",
			links: [
				{ path: "/simple", label: "Simple SSR", mode: "SSR", desc: "No loader, default props" },
				{ path: "/user/123", label: "Path Parameters", mode: "SSR", desc: "Dynamic URL segments" },
				{ path: "/search?q=hello", label: "Query Parameters", mode: "SSR", desc: "URL query strings" },
				{ path: "/shared-a", label: "Shared A", mode: "SSR", desc: "Same component, different route" },
				{ path: "/shared-b", label: "Shared B", mode: "SSR", desc: "Same component, different route" },
				{ path: "/empty", label: "Empty Props", mode: "SSR", desc: "Loader returns empty object" },
				{ path: "/nested", label: "Nested Path", mode: "SSR", desc: "File in subdirectory" },
				{ path: "/message/hello-world", label: "Dynamic Message", mode: "SSR", desc: "Extract path segments" },
			]
		},
		{
			title: "Static Pages",
			description: "Pre-rendered at build time",
			links: [
				{ path: "/product", label: "Product", mode: "Static", desc: "Static prerender without data" },
				{ path: "/blog/hello-world", label: "Blog: Hello World", mode: "Static", desc: "Static with dynamic data" },
				{ path: "/blog/getting-started", label: "Blog: Getting Started", mode: "Static", desc: "Static with dynamic data" },
			]
		},
		{
			title: "Client-Only",
			description: "No SSR, client-side hydration only",
			links: [
				{ path: "/client", label: "Client Basic", mode: "Client-Only", desc: "Basic client-only page" },
				{ path: "/client/deep", label: "Client Deep", mode: "Client-Only", desc: "Nested client-only path" },
			]
		},
		{
			title: "Authentication Demo",
			description: "RedirectError interface demonstration",
			links: [
				{ path: "/dashboard", label: "Dashboard (Redirect)", mode: "SSR", desc: "Redirects to /login" },
				{ path: "/dashboard?demo=true", label: "Dashboard (Auth)", mode: "SSR", desc: "Shows dashboard after login" },
			]
		},
		{
			title: "Error Scenarios",
			description: "Error handling demonstrations",
			links: [
				{ path: "/error", label: "Generic Error", mode: "Error", desc: "500 error page" },
				{ path: "/error-loader", label: "Loader Error", mode: "Error", desc: "Error in data loader" },
				{ path: "/error-render", label: "Render Error", mode: "Error", desc: "Error during component render" },
				{ path: "/error-import", label: "Import Error", mode: "Error", desc: "Module import failure" },
				{ path: "/error-redirect-302", label: "Redirect 302", mode: "Error", desc: "Temporary redirect" },
				{ path: "/error-redirect-307", label: "Redirect 307", mode: "Error", desc: "Temporary redirect (preserve method)" },
			]
		},
		{
			title: "API & Data",
			description: "Data loading demonstrations",
			links: [
				{ path: "/api-demo", label: "API Demo", mode: "SSR", desc: "Server-loaded user data" },
			]
		},
	];
	
	function getModeClasses(mode: string) {
		switch (mode) {
			case "SSR":
				return "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200";
			case "Static":
				return "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200";
			case "Client-Only":
				return "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200";
			case "Error":
				return "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200";
			default:
				return "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200";
		}
	}
</script>

<svelte:head>
	<title>Bifrost Svelte Examples - {name}</title>
	<meta name="description" content="Bifrost SSR framework Svelte examples and test pages" />
</svelte:head>

<Layout>
	<div class="max-w-6xl mx-auto px-4 py-8 md:py-12">
		<div class="text-center mb-12">
			<Hello {name} />
			<p class="text-muted-foreground text-lg mt-4">
				Bifrost SSR Framework - Svelte Test Pages Index
			</p>
			<div class="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-primary/10 rounded-full">
				<span class="text-primary font-medium">Powered by Svelte</span>
			</div>
		</div>

		<div class="grid gap-6">
			{#each sections as section}
				<div class="bg-card rounded-xl p-6 shadow-sm border border-border">
					<h2 class="text-xl font-semibold mb-2 text-card-foreground">{section.title}</h2>
					<p class="text-muted-foreground text-sm mb-4">{section.description}</p>

					<ul class="space-y-3">
						{#each section.links as link}
							<li class="flex flex-wrap items-center gap-3 py-3 border-b border-border last:border-0">
								<a href={link.path} class="text-primary hover:text-primary/80 font-medium min-w-[200px] hover:underline transition-all">
									{link.label}
								</a>
								<span class="px-3 py-1 rounded-full text-xs font-medium uppercase tracking-wide {getModeClasses(link.mode)}">
									{link.mode}
								</span>
								<code class="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">{link.path}</code>
								<span class="text-sm text-muted-foreground ml-auto">{link.desc}</span>
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		</div>
	</div>
</Layout>
