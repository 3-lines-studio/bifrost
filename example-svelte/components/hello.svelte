<script lang="ts">
	let { name }: { name: string } = $props();
</script>

<div class="flex items-center gap-2">
	<span class="text-4xl font-bold">Hello</span>
	<span class="text-4xl font-bold text-primary">{name}</span>
	<span class="text-4xl">!</span>
</div>
