<script lang="ts">
	import Layout from '../layout/base.svelte';

	interface NavItem {
		slug: string;
		title: string;
	}

	let { title, description, html, slug, nav }: {
		title: string;
		description: string;
		html: string;
		slug: string;
		nav: NavItem[];
	} = $props();
</script>

<svelte:head>
	<title>{title} — Bifrost</title>
	<meta name="description" content={description || `${title} — Bifrost documentation`} />
</svelte:head>

<Layout {nav}>
	<div class="max-w-4xl mx-auto px-6 py-12 lg:grid lg:grid-cols-[200px_1fr] lg:gap-12">
		<aside class="hidden lg:block">
			<nav class="sticky top-20">
				<ul class="space-y-1 text-sm">
					{#each nav as item}
						<li>
							<a
								href="/docs/{item.slug}"
								class="block py-1.5 transition-colors {item.slug === slug
									? 'text-accent font-medium'
									: 'text-muted hover:text-fg'}"
							>
								{item.title}
							</a>
						</li>
					{/each}
				</ul>
			</nav>
		</aside>

		<article class="min-w-0">
			<div class="mb-8">
				<h1 class="text-3xl font-bold tracking-tight mb-2">{title}</h1>
				{#if description}
					<p class="text-muted text-lg">{description}</p>
				{/if}
			</div>

			<div class="prose">
				{@html html}
			</div>

			<div class="mt-12 pt-8 border-t border-border flex justify-between text-sm">
				{#each nav as item, i}
					{#if item.slug === slug}
						{#if i > 0}
							<a href="/docs/{nav[i - 1].slug}" class="text-muted hover:text-fg transition-colors">
								&larr; {nav[i - 1].title}
							</a>
						{:else}
							<span></span>
						{/if}
						{#if i < nav.length - 1}
							<a href="/docs/{nav[i + 1].slug}" class="text-muted hover:text-fg transition-colors">
								{nav[i + 1].title} &rarr;
							</a>
						{/if}
					{/if}
				{/each}
			</div>
		</article>
	</div>
</Layout>
