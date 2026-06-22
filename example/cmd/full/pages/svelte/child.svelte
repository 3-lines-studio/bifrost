<script lang="ts">
  let { label, value }: { label: string; value: number } = $props();
</script>

<div class="flex items-center gap-3 py-2">
  <span class="text-muted-foreground text-sm">{label}</span>
  <span class="font-mono text-sm">{value}</span>
</div>
