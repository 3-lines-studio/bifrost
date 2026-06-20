<svelte:head>
  <title>Svelte Demo</title>
  <meta name="description" content="Svelte 5 + Tailwind CSS example page" />
  <script>
    (function() {
      const theme = localStorage.getItem('vite-ui-theme') || 'dark';
      const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      const isDark = theme === 'dark' || (theme === 'system' && systemDark);
      document.documentElement.classList.add(isDark ? 'dark' : 'light');
    })();
  </script>
</svelte:head>

<script lang="ts">
  import "../../style.css";
  import Child from "./child.svelte";
  import Card from "../../components/svelte/Card.svelte";

  let { name = "World" }: { name?: string } = $props();
  let count = $state(0);
</script>

<div class="max-w-6xl mx-auto px-4 py-8 md:py-12">
  <div class="text-center mb-12">
    <div class="flex items-center justify-center gap-4 mb-4">
      <div>Hello, {name}!</div>
    </div>
    <p class="text-muted-foreground text-lg">
      Svelte 5 + Tailwind CSS — SSR with Bifrost
    </p>
    <button
      onclick={() => count++}
      class="inline-flex items-center justify-center rounded-md bg-primary text-primary-foreground h-9 px-4 mt-4 hover:bg-primary/90 transition-colors"
    >
      Clicked {count} {count === 1 ? 'time' : 'times'}
    </button>
  </div>

  <div class="grid gap-6">
    <div class="bg-card rounded-xl p-6 shadow-sm border border-border">
      <h2 class="text-xl font-semibold mb-2 text-card-foreground">Cross-Framework Navigation</h2>
      <p class="text-muted-foreground text-sm mb-4">Same app, different frameworks coexisting</p>
      <ul class="space-y-3">
        <li class="flex flex-wrap items-center gap-3 py-3 border-b border-border last:border-0">
          <a href="/" class="text-primary hover:text-primary/80 font-medium min-w-[200px] hover:underline transition-all">React Home</a>
          <span class="px-3 py-1 rounded-full text-xs font-medium uppercase tracking-wide bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">SSR</span>
          <code class="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">/</code>
        </li>
        <li class="flex flex-wrap items-center gap-3 py-3 border-b border-border last:border-0">
          <a href="/about" class="text-primary hover:text-primary/80 font-medium min-w-[200px] hover:underline transition-all">React About</a>
          <span class="px-3 py-1 rounded-full text-xs font-medium uppercase tracking-wide bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">Client-Only</span>
          <code class="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">/about</code>
        </li>
        <li class="flex flex-wrap items-center gap-3 py-3 border-b border-border last:border-0">
          <a href="/svelte" class="text-primary hover:text-primary/80 font-medium min-w-[200px] hover:underline transition-all">Svelte (this page)</a>
          <span class="px-3 py-1 rounded-full text-xs font-medium uppercase tracking-wide bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">Svelte SSR</span>
          <code class="text-xs text-muted-foreground font-mono bg-muted px-2 py-1 rounded">/svelte</code>
        </li>
      </ul>
    </div>

    <div class="bg-card rounded-xl p-6 shadow-sm border border-border">
      <h2 class="text-xl font-semibold mb-2 text-card-foreground">Svelte 5 Runes</h2>
      <p class="text-muted-foreground text-sm mb-4">Interactive state with $state()</p>
      <div class="flex items-center gap-4">
        <button
          onclick={() => count++}
          class="inline-flex items-center justify-center rounded-md bg-secondary text-secondary-foreground h-9 px-4 hover:bg-secondary/80 transition-colors"
        >
          Increment
        </button>
        <button
          onclick={() => count = 0}
          class="inline-flex items-center justify-center rounded-md border border-border bg-transparent h-9 px-4 hover:bg-accent transition-colors"
        >
          Reset
        </button>
        <span class="text-2xl font-bold tabular-nums">{count}</span>
      </div>
    </div>

    <div class="bg-card rounded-xl p-6 shadow-sm border border-border">
      <h2 class="text-xl font-semibold mb-2 text-card-foreground">Nested Components</h2>
      <p class="text-muted-foreground text-sm mb-4">Child Svelte components imported and rendered</p>
      <Child label="Props from server" value={count} />
      <Child label="Static value" value={42} />
    </div>

    <Card title="Reusable Card Component" description="This card is a reusable Svelte component with scoped styles. It demonstrates cross-component composition with Svelte 5." />
  </div>
</div>
