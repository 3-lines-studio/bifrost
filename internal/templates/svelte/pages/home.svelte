<script lang="ts">
  import "../style.css";
  let { message }: { message: string } = $props();
</script>

<svelte:head>
  <title>Welcome to Bifrost</title>
  <meta name="description" content="Bifrost app" />
</svelte:head>

<div class="flex min-h-screen flex-col items-center justify-center gap-2">
  <h1 class="text-3xl font-bold">{message}</h1>
  <p class="text-gray-600">Your Bifrost app is running!</p>
</div>
