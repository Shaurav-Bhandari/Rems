<script lang="ts">
	import { auth } from '$lib/stores/auth';
	import { goto } from '$app/navigation';

	let email = $state('');
	let userName = $state('');
	let fullName = $state('');
	let password = $state('');
	let passwordConfirm = $state('');
	let tenantName = $state('');
	let phone = $state('');
	let loading = $state(false);
	let error = $state('');
	let success = $state(false);

	async function handleRegister() {
		error = '';

		if (password !== passwordConfirm) {
			error = 'Passwords do not match';
			return;
		}

		loading = true;

		const result = await auth.register({
			email,
			user_name: userName,
			password,
			password_confirm: passwordConfirm,
			full_name: fullName,
			tenant_name: tenantName,
			phone: phone || undefined
		});

		if (!result.success) {
			error = result.message || 'Registration failed';
			loading = false;
			return;
		}

		loading = false;
		success = true;

		setTimeout(() => goto('/login'), 2000);
	}
</script>

<svelte:head>
	<title>Register — ReMS</title>
	<meta name="description" content="Create a ReMS account" />
</svelte:head>

<div class="auth-page">
	<div class="auth-card" style="max-width: 500px;">
		<div class="text-center mb-4">
			<div class="logo mb-2">Re<span>MS</span></div>
			<p class="text-muted mb-0" style="font-size: 0.9rem;">Create your account</p>
		</div>

		{#if error}
			<div class="alert alert-danger d-flex align-items-center gap-2 py-2" role="alert">
				<i class="bi bi-exclamation-triangle-fill"></i>
				<span>{error}</span>
			</div>
		{/if}

		{#if success}
			<div class="alert d-flex align-items-center gap-2 py-2" style="background: rgba(191, 215, 181, 0.3); color: #2d6a2d;" role="alert">
				<i class="bi bi-check-circle-fill"></i>
				<span>Account created successfully! Redirecting to login...</span>
			</div>
		{:else}
			<form on:submit|preventDefault={handleRegister}>
				<div class="row">
					<div class="col-md-6 mb-3">
						<label for="fullName" class="form-label fw-medium" style="font-size: 0.85rem;">Full Name</label>
						<input type="text" class="form-control" id="fullName" bind:value={fullName}
							placeholder="John Doe" required />
					</div>
					<div class="col-md-6 mb-3">
						<label for="userName" class="form-label fw-medium" style="font-size: 0.85rem;">Username</label>
						<input type="text" class="form-control" id="userName" bind:value={userName}
							placeholder="johndoe" required />
					</div>
				</div>

				<div class="mb-3">
					<label for="regEmail" class="form-label fw-medium" style="font-size: 0.85rem;">Email Address</label>
					<input type="email" class="form-control" id="regEmail" bind:value={email}
						placeholder="you@example.com" required />
				</div>

				<div class="mb-3">
					<label for="regTenant" class="form-label fw-medium" style="font-size: 0.85rem;">Restaurant / Business Name</label>
					<input type="text" class="form-control" id="regTenant" bind:value={tenantName}
						placeholder="My Restaurant" required />
				</div>

				<div class="mb-3">
					<label for="regPhone" class="form-label fw-medium" style="font-size: 0.85rem;">Phone <span class="text-muted">(optional)</span></label>
					<input type="tel" class="form-control" id="regPhone" bind:value={phone}
						placeholder="+1234567890" />
				</div>

				<div class="row">
					<div class="col-md-6 mb-3">
						<label for="regPassword" class="form-label fw-medium" style="font-size: 0.85rem;">Password</label>
						<input type="password" class="form-control" id="regPassword" bind:value={password}
							placeholder="••••••••" required minlength="8" />
					</div>
					<div class="col-md-6 mb-3">
						<label for="regConfirm" class="form-label fw-medium" style="font-size: 0.85rem;">Confirm Password</label>
						<input type="password" class="form-control" id="regConfirm" bind:value={passwordConfirm}
							placeholder="••••••••" required minlength="8" />
					</div>
				</div>

				<button type="submit" class="btn btn-coral w-100 mb-3" disabled={loading}>
					{#if loading}
						<span class="spinner-border spinner-border-sm me-2" role="status"></span>
					{/if}
					Create Account
				</button>

				<p class="text-center text-muted mb-0" style="font-size: 0.85rem;">
					Already have an account?
					<a href="/login" class="text-decoration-none fw-semibold" style="color: var(--prussian-blue);">
						Sign in
					</a>
				</p>
			</form>
		{/if}
	</div>
</div>
