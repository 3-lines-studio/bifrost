<svelte:head>
  <title>Svelte Client Demo</title>
</svelte:head>

<script lang="ts">
  import "../../style.css";
  let count = $state(0);
</script>

<div class="max-w-6xl mx-auto px-4 py-8 md:py-12">
  <div class="text-center mb-12">
    <h1 class="text-3xl font-bold text-primary mb-2">Svelte Client-Only</h1>
    <p class="text-muted-foreground">This page renders entirely on the client</p>
  </div>
  <div class="bg-card rounded-xl p-6 shadow-sm border border-border max-w-md mx-auto">
    <div class="flex items-center justify-center gap-4">
      <button
        onclick={() => count--}
        class="inline-flex items-center justify-center rounded-md bg-secondary text-secondary-foreground h-9 w-9 hover:bg-secondary/80 transition-colors"
      >-</button>
      <span class="text-4xl font-bold tabular-nums">{count}</span>
      <button
        onclick={() => count++}
        class="inline-flex items-center justify-center rounded-md bg-primary text-primary-foreground h-9 w-9 hover:bg-primary/90 transition-colors"
      >+</button>
    </div>
  </div>
</div>
