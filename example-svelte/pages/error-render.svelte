<script lang="ts">
	import Layout from '../layout/base.svelte';
	
	// Intentionally throw an error during render
	throw new Error('This is a test render error');
</script>

<svelte:head>
	<title>Render Error</title>
</svelte:head>

<Layout>
	<p>This should never render</p>
</Layout>
