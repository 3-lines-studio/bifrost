<script lang="ts">
	import Layout from '../layout/base.svelte';
	
	let { user }: { user: { name: string; role: string } } = $props();
</script>

<svelte:head>
	<title>Dashboard</title>
</svelte:head>

<Layout>
	<div class="max-w-6xl mx-auto p-8">
		<div class="flex justify-between items-center mb-8">
			<h1 class="text-3xl font-bold">Dashboard</h1>
			<div class="flex items-center gap-4">
				<span class="text-muted-foreground">Welcome, {user.name}</span>
				<span class="px-3 py-1 bg-primary text-primary-foreground rounded-full text-sm">{user.role}</span>
			</div>
		</div>
		
		<div class="grid grid-cols-1 md:grid-cols-3 gap-6">
			<div class="bg-card p-6 rounded-xl border">
				<h3 class="text-lg font-semibold mb-2">Total Users</h3>
				<p class="text-3xl font-bold text-primary">1,234</p>
			</div>
			<div class="bg-card p-6 rounded-xl border">
				<h3 class="text-lg font-semibold mb-2">Revenue</h3>
				<p class="text-3xl font-bold text-green-600">$12,345</p>
			</div>
			<div class="bg-card p-6 rounded-xl border">
				<h3 class="text-lg font-semibold mb-2">Orders</h3>
				<p class="text-3xl font-bold text-blue-600">567</p>
			</div>
		</div>
		
		<div class="mt-8">
			<a href="/" class="text-primary hover:underline">← Back to Home</a>
		</div>
	</div>
</Layout>
