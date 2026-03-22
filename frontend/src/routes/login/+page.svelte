<script lang="ts">
	import { auth } from '$lib/stores/auth';
	import { goto } from '$app/navigation';

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let error = $state('');

	// 2FA state
	let show2FA = $state(false);
	let pendingSessionId = $state('');
	let twoFACode = $state('');

	async function handleLogin() {
		error = '';
		loading = true;

		const result = await auth.login(email, password);

		if (!result.success && !result.requires2FA) {
			error = result.message || 'Login failed';
			loading = false;
			return;
		}

		if (result.requires2FA) {
			show2FA = true;
			pendingSessionId = result.pendingSessionId || '';
			loading = false;
			return;
		}

		loading = false;
		goto('/dashboard');
	}

	async function handle2FA() {
		error = '';
		loading = true;
		// 2FA verification would go through the API
		const { api } = await import('$lib/api/client');
		const res = await api.post('/auth/verify-2fa', {
			pending_session_id: pendingSessionId,
			code: twoFACode
		});

		if (!res.success) {
			error = res.message || '2FA verification failed';
			loading = false;
			return;
		}

		loading = false;
		goto('/dashboard');
	}
</script>

<svelte:head>
	<title>Login — ReMS</title>
	<meta name="description" content="Sign in to ReMS Restaurant Management System" />
</svelte:head>

<div class="auth-page">
	<div class="auth-card">
		<div class="text-center mb-4">
			<div class="logo mb-2">Re<span>MS</span></div>
			<p class="text-muted mb-0" style="font-size: 0.9rem;">Restaurant Management System</p>
		</div>

		{#if error}
			<div class="alert alert-danger d-flex align-items-center gap-2 py-2" role="alert">
				<i class="bi bi-exclamation-triangle-fill"></i>
				<span>{error}</span>
			</div>
		{/if}

		{#if !show2FA}
			<form onsubmit={(e) => { e.preventDefault(); handleLogin(); }}>
				<div class="mb-3">
					<label for="email" class="form-label fw-medium" style="font-size: 0.85rem;">Email Address</label>
					<div class="input-group">
						<span class="input-group-text" style="border-radius: 10px 0 0 10px; border-right: none; background: #f8f6f2;">
							<i class="bi bi-envelope" style="color: var(--taupe-grey);"></i>
						</span>
						<input
							type="email"
							class="form-control"
							id="email"
							bind:value={email}
							placeholder="you@example.com"
							required
							style="border-left: none; border-radius: 0 10px 10px 0;"
						/>
					</div>
				</div>

				<div class="mb-4">
					<label for="password" class="form-label fw-medium" style="font-size: 0.85rem;">Password</label>
					<div class="input-group">
						<span class="input-group-text" style="border-radius: 10px 0 0 10px; border-right: none; background: #f8f6f2;">
							<i class="bi bi-lock" style="color: var(--taupe-grey);"></i>
						</span>
						<input
							type="password"
							class="form-control"
							id="password"
							bind:value={password}
							placeholder="••••••••"
							required
							style="border-left: none; border-radius: 0 10px 10px 0;"
						/>
					</div>
					<div class="text-end mt-2">
						<a href="/forgot-password" class="text-decoration-none" style="font-size: 0.82rem; color: var(--vibrant-coral);">
							Forgot password?
						</a>
					</div>
				</div>

				<button type="submit" class="btn btn-coral w-100 mb-3" disabled={loading}>
					{#if loading}
						<span class="spinner-border spinner-border-sm me-2" role="status"></span>
					{/if}
					Sign In
				</button>

				<p class="text-center text-muted mb-0" style="font-size: 0.85rem;">
					Don't have an account?
					<a href="/register" class="text-decoration-none fw-semibold" style="color: var(--prussian-blue);">
						Create one
					</a>
				</p>
			</form>
		{:else}
			<form onsubmit={(e) => { e.preventDefault(); handle2FA(); }}>
				<div class="text-center mb-3">
					<div class="rounded-circle d-inline-flex align-items-center justify-content-center mb-3"
						style="width: 60px; height: 60px; background: rgba(248, 112, 96, 0.1);">
						<i class="bi bi-shield-lock" style="font-size: 1.5rem; color: var(--vibrant-coral);"></i>
					</div>
					<h5 class="fw-bold" style="color: var(--prussian-blue);">Two-Factor Authentication</h5>
					<p class="text-muted" style="font-size: 0.85rem;">Enter the code from your authenticator app</p>
				</div>

				<div class="mb-4">
					<input
						type="text"
						class="form-control text-center"
						bind:value={twoFACode}
						placeholder="000000"
						maxlength="6"
						required
						style="font-size: 1.5rem; letter-spacing: 8px; font-weight: 700;"
					/>
				</div>

				<button type="submit" class="btn btn-coral w-100 mb-3" disabled={loading}>
					{#if loading}
						<span class="spinner-border spinner-border-sm me-2" role="status"></span>
					{/if}
					Verify
				</button>

				<button type="button" class="btn btn-outline-secondary w-100" onclick={() => { show2FA = false; twoFACode = ''; }}>
					Back to Login
				</button>
			</form>
		{/if}
	</div>
</div>
