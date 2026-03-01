<script lang="ts">
	import Layout from '../layout/base.svelte';
</script>

<svelte:head>
	<title>About</title>
</svelte:head>

<Layout>
	<div class="max-w-3xl mx-auto p-8">
		<h1 class="text-3xl font-bold mb-4">About</h1>
		<p class="text-muted-foreground">
			This is a Svelte client-only page. It was rendered entirely on the client.
		</p>
		<div class="mt-8">
			<a href="/" class="text-primary hover:underline">← Back to Home</a>
		</div>
	</div>
</Layout>
