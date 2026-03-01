<script lang="ts">
	import Layout from '../layout/base.svelte';
	
	let { users, loadTime }: { users: Array<{ id: number; name: string; email: string }>; loadTime: string } = $props();
</script>

<svelte:head>
	<title>API Demo</title>
</svelte:head>

<Layout>
	<div class="max-w-4xl mx-auto p-8">
		<h1 class="text-3xl font-bold mb-2">API Demo</h1>
		<p class="text-muted-foreground mb-8">Data loaded at: {loadTime}</p>
		
		<div class="bg-card rounded-xl border overflow-hidden">
			<table class="w-full">
				<thead class="bg-muted">
					<tr>
						<th class="px-6 py-3 text-left text-sm font-medium">ID</th>
						<th class="px-6 py-3 text-left text-sm font-medium">Name</th>
						<th class="px-6 py-3 text-left text-sm font-medium">Email</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#each users as user}
						<tr class="hover:bg-muted/50">
							<td class="px-6 py-4">{user.id}</td>
							<td class="px-6 py-4">{user.name}</td>
							<td class="px-6 py-4 text-muted-foreground">{user.email}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		
		<div class="mt-8">
			<a href="/" class="text-primary hover:underline">← Back to Home</a>
		</div>
	</div>
</Layout>
