<script lang="ts">
	import Layout from '../layout/base.svelte';
</script>

<svelte:head>
	<title>Login</title>
</svelte:head>

<Layout>
	<div class="max-w-md mx-auto p-8">
		<div class="bg-card rounded-xl p-8 shadow-sm border border-border">
			<h1 class="text-2xl font-bold mb-6 text-center">Login</h1>
			
			<form class="space-y-4">
				<div>
					<label class="block text-sm font-medium mb-1">Email</label>
					<input 
						type="email" 
						class="w-full px-3 py-2 border rounded-md bg-background"
						placeholder="Enter your email"
					/>
				</div>
				<div>
					<label class="block text-sm font-medium mb-1">Password</label>
					<input 
						type="password" 
						class="w-full px-3 py-2 border rounded-md bg-background"
						placeholder="Enter your password"
					/>
				</div>
				<button 
					type="submit" 
					class="w-full bg-primary text-primary-foreground py-2 rounded-md hover:bg-primary/90"
				>
					Sign In
				</button>
			</form>
		</div>
		
		<div class="mt-8 text-center">
			<a href="/" class="text-primary hover:underline">← Back to Home</a>
		</div>
	</div>
</Layout>
