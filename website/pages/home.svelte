<script lang="ts">
	import Layout from '../layout/base.svelte';

	interface NavItem {
		slug: string;
		title: string;
	}

	let { nav }: { nav: NavItem[] } = $props();

	const features = [
		{
			title: 'Multi-Framework',
			desc: 'React and Svelte with identical Go APIs.'
		},
		{
			title: 'Server-Side Rendering',
			desc: 'Full SSR with data loading from Go handlers.'
		},
		{
			title: 'Static Generation',
			desc: 'Prerender pages at build time. No runtime needed.'
		},
		{
			title: 'Single Binary',
			desc: 'Embed everything into one Go binary for deployment.'
		},
		{
			title: 'Hot Reload',
			desc: 'Instant feedback during development.'
		},
		{
			title: 'Bun-Powered',
			desc: 'Fast builds and SSR powered by Bun.'
		}
	];
</script>

<svelte:head>
	<title>Bifrost — SSR for Go</title>
	<meta name="description" content="Server-side rendering for React and Svelte components in Go. Bridge your backend with modern frontends." />
</svelte:head>

<Layout {nav}>
	<section class="max-w-4xl mx-auto px-6 pt-24 pb-16">
		<div class="max-w-2xl">
			<h1 class="text-4xl sm:text-5xl font-bold tracking-tight leading-[1.1] mb-6">
				Server-side rendering<br />for <span class="text-accent">Go</span>
			</h1>
			<p class="text-lg text-muted leading-relaxed mb-10 max-w-lg">
				Bridge your Go backend with React and Svelte frontends.
				SSR, static generation, and single-binary deployment.
			</p>

			<div class="flex flex-wrap gap-3">
				<a href="/docs/getting-started"
					class="inline-flex items-center px-5 py-2.5 bg-fg text-bg rounded-lg text-sm font-medium hover:opacity-90 transition-opacity">
					Get Started
				</a>
				<div class="flex items-center gap-2 px-4 py-2.5 bg-code-bg border border-border rounded-lg text-sm font-mono text-muted">
					<span>go get github.com/3-lines-studio/bifrost</span>
				</div>
			</div>
		</div>
	</section>

	<section class="max-w-4xl mx-auto px-6 pb-24">
		<div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-6">
			{#each features as feature}
				<div class="p-5 rounded-xl border border-border bg-surface">
					<h3 class="font-semibold mb-1.5 text-fg">{feature.title}</h3>
					<p class="text-sm text-muted leading-relaxed">{feature.desc}</p>
				</div>
			{/each}
		</div>
	</section>

	<section class="border-t border-border">
		<div class="max-w-4xl mx-auto px-6 py-16">
			<h2 class="text-2xl font-semibold tracking-tight mb-8">Quick Start</h2>
			<div class="bg-code-bg border border-border rounded-xl p-6 overflow-x-auto">
				<pre class="text-sm leading-relaxed"><code>package main

import (
    "embed"
    "log"
    "net/http"

    "github.com/3-lines-studio/bifrost"
)

//go:embed all:.bifrost
var bifrostFS embed.FS

func main() &#123;
    app := bifrost.New(
        bifrostFS,
        bifrost.Page("/", "./pages/home.tsx",
            bifrost.WithLoader(func(req *http.Request) (map[string]any, error) &#123;
                return map[string]any&#123;"name": "World"&#125;, nil
            &#125;),
        ),
    )
    defer app.Stop()

    log.Fatal(http.ListenAndServe(":8080", app.Handler()))
&#125;</code></pre>
			</div>
		</div>
	</section>
</Layout>
