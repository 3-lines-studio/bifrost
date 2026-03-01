<script lang="ts">
	import Layout from '../../layout/base.svelte';
	
	let { name }: { name: string } = $props();
</script>

<svelte:head>
	<title>Nested Page</title>
</svelte:head>

<Layout>
	<div class="max-w-3xl mx-auto p-8">
		<h1 class="text-3xl font-bold mb-4">Nested Page: {name}</h1>
		<p class="text-muted-foreground mb-4">
			This page is located at <code class="bg-muted px-2 py-1 rounded">pages/nested/page.svelte</code>
		</p>
		<div class="bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200 p-4 rounded-lg">
			Success! Nested paths work with Svelte.
		</div>
		<div class="mt-8">
			<a href="/" class="text-primary hover:underline">← Back to Home</a>
		</div>
	</div>
</Layout>
