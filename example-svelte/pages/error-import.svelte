<script lang="ts">
	import Layout from '../layout/base.svelte';
	// Import a non-existent module to trigger an import error
	import NonExistent from './non-existent-module';
</script>

<svelte:head>
	<title>Import Error</title>
</svelte:head>

<Layout>
	<p>This should never render</p>
</Layout>
