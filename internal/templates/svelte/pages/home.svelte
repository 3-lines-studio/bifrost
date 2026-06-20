<script lang="ts">
  let { message }: { message: string } = $props();
</script>

<svelte:head>
  <title>Welcome to Bifrost</title>
  <meta name="description" content="Bifrost app" />
</svelte:head>

<div style="padding: 2rem; font-family: system-ui, sans-serif;">
  <h1>{message}</h1>
  <p>Your Bifrost app is running!</p>
</div>
