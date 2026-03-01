<script lang="ts">
	import Layout from '../layout/base.svelte';
	
	let { slug, title, body }: { slug: string; title: string; body: string } = $props();
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={`Blog post: ${title}`} />
</svelte:head>

<Layout>
	<article class="max-w-3xl mx-auto p-8">
		<h1 class="text-3xl font-bold mb-4">{title}</h1>
		<div class="text-muted-foreground mb-8">Slug: {slug}</div>
		<div class="prose dark:prose-invert max-w-none">
			<p class="text-lg leading-relaxed">{body}</p>
		</div>
		<div class="mt-8 pt-8 border-t">
			<a href="/" class="text-primary hover:underline">← Back to Home</a>
		</div>
	</article>
</Layout>
