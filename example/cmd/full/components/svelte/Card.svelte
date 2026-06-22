<script lang="ts">
  let { title = "Card", description = "" }: { title?: string; description?: string } = $props();
</script>

<div class="bg-card rounded-xl p-6 shadow-sm border border-border">
  <h2 class="text-xl font-semibold mb-2 text-card-foreground">{title}</h2>
  <p class="text-muted-foreground text-sm">{description}</p>
</div>

<style>
  div {
    transition: box-shadow 0.2s ease;
  }
  div:hover {
    box-shadow: 0 4px 12px rgba(0,0,0,0.1);
  }
</style>
