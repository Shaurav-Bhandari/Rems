<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { isAuthenticated } from '$lib/stores/auth';

	onMount(() => {
		const unsub = isAuthenticated.subscribe((val) => {
			if (val) {
				goto('/dashboard');
			} else {
				goto('/login');
			}
		});
		return unsub;
	});
</script>

<div class="auth-page">
	<div class="loading-spinner">
		<div class="spinner-border" role="status">
			<span class="visually-hidden">Loading...</span>
		</div>
	</div>
</div>
