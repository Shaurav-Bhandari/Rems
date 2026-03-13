<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { auth, currentUser } from '$lib/stores/auth';

	let activeTab = $state<'profile' | 'password' | 'sessions'>('profile');

	// Password change
	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let pwdLoading = $state(false);
	let pwdError = $state('');
	let pwdSuccess = $state('');

	// Sessions
	let sessions = $state<any[]>([]);
	let sessionsLoading = $state(false);

	onMount(async () => { await loadSessions(); });

	async function loadSessions() {
		sessionsLoading = true;
		const res = await api.get<any>('/auth/sessions');
		if (res.success && res.data) {
			sessions = res.data.sessions || [];
		}
		sessionsLoading = false;
	}

	async function handleChangePassword() {
		pwdError = '';
		pwdSuccess = '';

		if (newPassword !== confirmPassword) {
			pwdError = 'Passwords do not match';
			return;
		}

		pwdLoading = true;
		const res = await api.post('/auth/change-password', {
			current_password: currentPassword,
			new_password: newPassword,
			confirm_password: confirmPassword
		});

		if (!res.success) {
			pwdError = res.message || 'Failed to change password';
			pwdLoading = false;
			return;
		}

		pwdSuccess = 'Password changed successfully';
		currentPassword = '';
		newPassword = '';
		confirmPassword = '';
		pwdLoading = false;
	}

	async function revokeSession(sessionId: string) {
		if (!confirm('Revoke this session?')) return;
		await api.del(`/auth/sessions/${sessionId}`);
		await loadSessions();
	}
</script>

<svelte:head><title>Settings — ReMS</title></svelte:head>

<div class="page-header">
	<h1>Settings</h1>
</div>

<div class="page-content fade-in">
	<div class="row">
		<!-- Tab Nav -->
		<div class="col-md-3 mb-3">
			<div class="card">
				<div class="card-body p-2">
					<div class="d-flex flex-column gap-1">
						<button class="btn text-start" class:btn-prussian={activeTab === 'profile'} class:btn-light={activeTab !== 'profile'}
							on:click={() => activeTab = 'profile'}>
							<i class="bi bi-person me-2"></i> Profile
						</button>
						<button class="btn text-start" class:btn-prussian={activeTab === 'password'} class:btn-light={activeTab !== 'password'}
							on:click={() => activeTab = 'password'}>
							<i class="bi bi-shield-lock me-2"></i> Password
						</button>
						<button class="btn text-start" class:btn-prussian={activeTab === 'sessions'} class:btn-light={activeTab !== 'sessions'}
							on:click={() => activeTab = 'sessions'}>
							<i class="bi bi-phone me-2"></i> Sessions
						</button>
					</div>
				</div>
			</div>
		</div>

		<!-- Tab Content -->
		<div class="col-md-9">
			{#if activeTab === 'profile'}
				<div class="card">
					<div class="card-body">
						<h5 class="fw-bold mb-4" style="color: var(--prussian-blue);">Profile Information</h5>
						{#if $currentUser}
							<div class="row g-3">
								<div class="col-md-6">
									<label class="form-label fw-medium text-muted" style="font-size: 0.82rem;">Full Name</label>
									<div class="form-control-plaintext fw-medium">{$currentUser.full_name}</div>
								</div>
								<div class="col-md-6">
									<label class="form-label fw-medium text-muted" style="font-size: 0.82rem;">Email</label>
									<div class="form-control-plaintext fw-medium">{$currentUser.email}</div>
								</div>
								<div class="col-md-6">
									<label class="form-label fw-medium text-muted" style="font-size: 0.82rem;">Role</label>
									<div>
										<span class="badge" style="background: rgba(193, 238, 255, 0.2); color: var(--prussian-blue); padding: 0.4rem 0.8rem; border-radius: 8px;">
											{$currentUser.default_role}
										</span>
									</div>
								</div>
								<div class="col-md-6">
									<label class="form-label fw-medium text-muted" style="font-size: 0.82rem;">Organization</label>
									<div class="form-control-plaintext fw-medium">{$currentUser.tenant_name}</div>
								</div>
								<div class="col-md-6">
									<label class="form-label fw-medium text-muted" style="font-size: 0.82rem;">User ID</label>
									<div class="form-control-plaintext text-muted" style="font-size: 0.82rem; font-family: monospace;">
										{$currentUser.user_id}
									</div>
								</div>
								<div class="col-md-6">
									<label class="form-label fw-medium text-muted" style="font-size: 0.82rem;">Tenant ID</label>
									<div class="form-control-plaintext text-muted" style="font-size: 0.82rem; font-family: monospace;">
										{$currentUser.tenant_id}
									</div>
								</div>
							</div>
						{/if}
					</div>
				</div>

			{:else if activeTab === 'password'}
				<div class="card">
					<div class="card-body">
						<h5 class="fw-bold mb-4" style="color: var(--prussian-blue);">Change Password</h5>

						{#if pwdError}
							<div class="alert alert-danger py-2">{pwdError}</div>
						{/if}
						{#if pwdSuccess}
							<div class="alert py-2" style="background: rgba(191, 215, 181, 0.3); color: #2d6a2d;">{pwdSuccess}</div>
						{/if}

						<form on:submit|preventDefault={handleChangePassword} style="max-width: 400px;">
							<div class="mb-3">
								<label class="form-label fw-medium" style="font-size: 0.85rem;">Current Password</label>
								<input type="password" class="form-control" bind:value={currentPassword} required />
							</div>
							<div class="mb-3">
								<label class="form-label fw-medium" style="font-size: 0.85rem;">New Password</label>
								<input type="password" class="form-control" bind:value={newPassword} required minlength="8" />
								<div class="form-text">Minimum 12 characters with upper, lower, number, and special character.</div>
							</div>
							<div class="mb-4">
								<label class="form-label fw-medium" style="font-size: 0.85rem;">Confirm New Password</label>
								<input type="password" class="form-control" bind:value={confirmPassword} required />
							</div>
							<button type="submit" class="btn btn-coral" disabled={pwdLoading}>
								{#if pwdLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}
								Update Password
							</button>
						</form>
					</div>
				</div>

			{:else if activeTab === 'sessions'}
				<div class="card">
					<div class="card-body">
						<h5 class="fw-bold mb-4" style="color: var(--prussian-blue);">Active Sessions</h5>

						{#if sessionsLoading}
							<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
						{:else if sessions.length === 0}
							<div class="empty-state"><i class="bi bi-phone"></i><p>No active sessions</p></div>
						{:else}
							<div class="d-flex flex-column gap-2">
								{#each sessions as session}
									<div class="d-flex align-items-center justify-content-between p-3 rounded-3" style="background: #f8f6f2;">
										<div class="d-flex align-items-center gap-3">
											<div class="rounded-circle d-flex align-items-center justify-content-center"
												style="width: 40px; height: 40px; background: {session.is_current ? 'var(--prussian-blue)' : 'var(--alabaster-grey)'}; color: {session.is_current ? '#fff' : 'var(--carbon-black)'};">
												<i class="bi bi-phone"></i>
											</div>
											<div>
												<div class="fw-medium" style="font-size: 0.9rem;">
													Session {session.session_id?.substring(0, 8) || '—'}
													{#if session.is_current}
														<span class="badge ms-1" style="background: rgba(191, 215, 181, 0.3); color: #2d6a2d; font-size: 0.7rem;">Current</span>
													{/if}
												</div>
											</div>
										</div>
										{#if !session.is_current}
											<button class="btn btn-sm btn-outline-danger" on:click={() => revokeSession(session.session_id)}>
												Revoke
											</button>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>
