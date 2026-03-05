<script lang="ts">
	import '../style.css';

	interface NavItem {
		slug: string;
		title: string;
	}

	let { children, nav = [] }: { children: any; nav?: NavItem[] } = $props();
</script>

<div class="min-h-screen flex flex-col">
	<header class="border-b border-border sticky top-0 bg-bg/80 backdrop-blur-sm z-50">
		<div class="max-w-4xl mx-auto px-6 h-14 flex items-center justify-between">
			<a href="/" class="text-fg font-semibold tracking-tight text-lg hover:opacity-80 transition-opacity">
				bifrost
			</a>
			<nav class="flex items-center gap-6 text-sm">
				<a href="/docs/getting-started" class="text-muted hover:text-fg transition-colors">Docs</a>
				<a href="https://github.com/3-lines-studio/bifrost"
					class="text-muted hover:text-fg transition-colors"
					target="_blank" rel="noopener noreferrer">GitHub</a>
			</nav>
		</div>
	</header>

	<main class="flex-1">
		{@render children()}
	</main>

	<footer class="border-t border-border">
		<div class="max-w-4xl mx-auto px-6 py-8 text-sm text-muted">
			<p>MIT License &middot; Built with Bifrost</p>
		</div>
	</footer>
</div>
