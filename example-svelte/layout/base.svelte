<script lang="ts">
	import '../style.css';
	import { setContext } from 'svelte';
	import { themeScript } from '../lib/theme-script';
	
	let { children } = $props();
	
	let theme = $state('dark');
	
	setContext('theme', {
		get theme() { return theme; },
		setTheme: (t: string) => { theme = t; }
	});
</script>

<svelte:head>
	<script>{themeScript}</script>
</svelte:head>

<div class={theme}>
	<div class="min-h-screen bg-background text-foreground">
		{@render children()}
	</div>
</div>
