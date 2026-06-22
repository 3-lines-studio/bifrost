<svelte:head>
  <title>Svelte Static Demo</title>
</svelte:head>

<script lang="ts">
  import "../../style.css";
</script>

<div class="max-w-3xl mx-auto px-4 py-8 md:py-12">
  <div class="text-center mb-8">
    <h1 class="text-3xl font-bold text-primary mb-2">Svelte Static</h1>
    <p class="text-muted-foreground">This page is prerendered at build time</p>
  </div>
  <div class="bg-card rounded-xl p-6 shadow-sm border border-border">
    <p class="text-card-foreground text-lg">
      Static Svelte page with zero JavaScript overhead after build.
    </p>
  </div>
</div>
