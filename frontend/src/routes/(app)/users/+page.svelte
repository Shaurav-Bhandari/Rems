<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	let users = $state<any[]>([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let formError = $state('');
	let formSuccess = $state('');
	let formLoading = $state(false);
	let searchQuery = $state('');

	const availableRoles = ['superadmin', 'owner', 'manager', 'waiter', 'cashier', 'kitchen'];

	let newUser = $state({
		user_name: '',
		full_name: '',
		email: '',
		phone: '',
		password: '',
		role_name: 'waiter'
	});

	// Role guard — redirect non-superadmins
	onMount(() => {
		const unsub = currentUser.subscribe((u) => {
			if (u && u.default_role !== 'superadmin') {
				goto('/dashboard');
			}
		});

		loadUsers();
		return unsub;
	});

	async function loadUsers() {
		loading = true;
		const res = await api.get<any>('/users');
		if (res.success && res.data) {
			users = Array.isArray(res.data) ? res.data : res.data?.users || [];
		}
		loading = false;
	}

	async function handleCreate() {
		formLoading = true;
		formError = '';
		formSuccess = '';

		if (!newUser.user_name || !newUser.full_name || !newUser.email || !newUser.password) {
			formError = 'Please fill in all required fields';
			formLoading = false;
			return;
		}

		let tenantId = '';
		const unsub = currentUser.subscribe((u) => {
			if (u) tenantId = u.tenant_id;
		});
		unsub();

		const res = await api.post('/initialize/users', {
			tenant_id: tenantId,
			user_name: newUser.user_name,
			full_name: newUser.full_name,
			email: newUser.email,
			phone: newUser.phone,
			password: newUser.password,
			role_name: newUser.role_name
		});

		if (!res.success) {
			formError = res.message || 'Failed to create user';
			formLoading = false;
			return;
		}

		formSuccess = `User "${newUser.user_name}" created successfully`;
		newUser = { user_name: '', full_name: '', email: '', phone: '', password: '', role_name: 'waiter' };
		formLoading = false;
		showCreate = false;
		await loadUsers();
	}

	async function handleDelete(userId: string, userName: string) {
		if (!confirm(`Are you sure you want to delete user "${userName}"? This action cannot be undone.`)) return;
		const res = await api.del(`/users/${userId}`);
		if (res.success) {
			await loadUsers();
		}
	}

	function getFilteredUsers() {
		if (!searchQuery) return users;
		const q = searchQuery.toLowerCase();
		return users.filter(
			(u: any) =>
				u.full_name?.toLowerCase().includes(q) ||
				u.email?.toLowerCase().includes(q) ||
				u.user_name?.toLowerCase().includes(q) ||
				u.primary_role?.toLowerCase().includes(q)
		);
	}

	function getRoleBadgeStyle(role: string) {
		const styles: Record<string, string> = {
			superadmin: 'background: rgba(248, 112, 96, 0.15); color: #c94535;',
			owner: 'background: rgba(65, 60, 88, 0.15); color: #413C58;',
			manager: 'background: rgba(16, 37, 66, 0.15); color: #102542;',
			waiter: 'background: rgba(191, 215, 181, 0.3); color: #3a7d3a;',
			cashier: 'background: rgba(193, 238, 255, 0.3); color: #0d5e82;',
			kitchen: 'background: rgba(242, 231, 201, 0.5); color: #8a7340;'
		};
		return styles[role?.toLowerCase()] || 'background: rgba(205, 215, 214, 0.3); color: #655356;';
	}
</script>

<svelte:head><title>User Management — ReMS</title></svelte:head>

<div class="page-header">
	<div>
		<h1>User Management</h1>
		<p class="text-muted mb-0" style="font-size: 0.85rem;">
			Create and manage system users
		</p>
	</div>
	<button id="btn-add-user" class="btn btn-coral" on:click={() => { showCreate = true; formError = ''; formSuccess = ''; }}>
		<i class="bi bi-person-plus me-1"></i> Add User
	</button>
</div>

<div class="page-content fade-in">
	<!-- Search -->
	<div class="mb-3" style="max-width: 400px;">
		<div class="position-relative">
			<i class="bi bi-search position-absolute" style="left: 12px; top: 50%; transform: translateY(-50%); color: var(--taupe-grey);"></i>
			<input
				id="input-search-users"
				type="text"
				class="form-control"
				placeholder="Search users by name, email, or role..."
				bind:value={searchQuery}
				style="padding-left: 2.2rem;"
			/>
		</div>
	</div>

	<!-- Stats Row -->
	<div class="row g-3 mb-4">
		<div class="col-md-4 col-xl-3">
			<div class="kpi-card prussian">
				<div class="kpi-label">Total Users</div>
				<div class="kpi-value">{users.length}</div>
				<div class="kpi-change">
					<i class="bi bi-people me-1"></i> All registered users
				</div>
			</div>
		</div>
		<div class="col-md-4 col-xl-3">
			<div class="kpi-card grape">
				<div class="kpi-label">Roles Active</div>
				<div class="kpi-value">{new Set(users.map((u: any) => u.primary_role).filter(Boolean)).size}</div>
				<div class="kpi-change">
					<i class="bi bi-shield-check me-1"></i> Distinct roles in use
				</div>
			</div>
		</div>
		<div class="col-md-4 col-xl-3">
			<div class="kpi-card coral">
				<div class="kpi-label">Search Results</div>
				<div class="kpi-value">{getFilteredUsers().length}</div>
				<div class="kpi-change">
					<i class="bi bi-funnel me-1"></i> Matching users
				</div>
			</div>
		</div>
	</div>

	{#if formSuccess && !showCreate}
		<div class="alert py-2 mb-3" style="background: rgba(191, 215, 181, 0.3); color: #2d6a2d; border-radius: 12px;">
			<i class="bi bi-check-circle me-1"></i> {formSuccess}
		</div>
	{/if}

	{#if loading}
		<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
	{:else if getFilteredUsers().length === 0}
		<div class="empty-state">
			<i class="bi bi-people"></i>
			<p>{searchQuery ? 'No users match your search' : 'No users found'}</p>
		</div>
	{:else}
		<div class="table-container">
			<div class="table-responsive">
				<table class="table" id="table-users">
					<thead>
						<tr>
							<th>User</th>
							<th>Email</th>
							<th>Role</th>
							<th>Phone</th>
							<th>Created</th>
							<th style="width: 80px;">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each getFilteredUsers() as user}
							<tr>
								<td>
									<div class="d-flex align-items-center gap-2">
										<div class="rounded-circle d-flex align-items-center justify-content-center"
											style="width: 36px; height: 36px; min-width: 36px; background: var(--prussian-blue); color: #fff; font-weight: 600; font-size: 0.82rem;">
											{(user.full_name || user.user_name || '?').charAt(0).toUpperCase()}
										</div>
										<div>
											<div class="fw-medium" style="font-size: 0.9rem;">{user.full_name || user.user_name || '—'}</div>
											<div class="text-muted" style="font-size: 0.78rem;">@{user.user_name || '—'}</div>
										</div>
									</div>
								</td>
								<td style="font-size: 0.88rem;">{user.email || '—'}</td>
								<td>
									<span class="badge-status" style="{getRoleBadgeStyle(user.primary_role || user.default_role)}">
										{user.primary_role || user.default_role || '—'}
									</span>
								</td>
								<td class="text-muted" style="font-size: 0.85rem;">{user.phone || '—'}</td>
								<td class="text-muted" style="font-size: 0.82rem;">
									{user.created_at ? new Date(user.created_at).toLocaleDateString() : '—'}
								</td>
								<td>
									{#if (user.primary_role || user.default_role) !== 'superadmin'}
										<button
											class="btn btn-sm btn-outline-danger"
											title="Delete user"
											on:click={() => handleDelete(user.user_id || user.id, user.full_name || user.user_name)}
										>
											<i class="bi bi-trash"></i>
										</button>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>

<!-- Create User Modal -->
{#if showCreate}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showCreate = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered">
			<div class="modal-content">
				<div class="modal-header">
					<h5 class="modal-title"><i class="bi bi-person-plus me-2"></i>Add New User</h5>
					<button type="button" class="btn-close" on:click={() => showCreate = false}></button>
				</div>
				<div class="modal-body">
					{#if formError}<div class="alert alert-danger py-2">{formError}</div>{/if}
					<form on:submit|preventDefault={handleCreate}>
						<div class="row g-3">
							<div class="col-md-6">
								<label for="input-username" class="form-label fw-medium" style="font-size: 0.85rem;">
									Username <span class="text-danger">*</span>
								</label>
								<input id="input-username" type="text" class="form-control" bind:value={newUser.user_name} required placeholder="e.g. john_doe" />
							</div>
							<div class="col-md-6">
								<label for="input-fullname" class="form-label fw-medium" style="font-size: 0.85rem;">
									Full Name <span class="text-danger">*</span>
								</label>
								<input id="input-fullname" type="text" class="form-control" bind:value={newUser.full_name} required placeholder="e.g. John Doe" />
							</div>
							<div class="col-md-6">
								<label for="input-email" class="form-label fw-medium" style="font-size: 0.85rem;">
									Email <span class="text-danger">*</span>
								</label>
								<input id="input-email" type="email" class="form-control" bind:value={newUser.email} required placeholder="john@example.com" />
							</div>
							<div class="col-md-6">
								<label for="input-phone" class="form-label fw-medium" style="font-size: 0.85rem;">Phone</label>
								<input id="input-phone" type="tel" class="form-control" bind:value={newUser.phone} placeholder="+977-98XXXXXXXX" />
							</div>
							<div class="col-md-6">
								<label for="input-password" class="form-label fw-medium" style="font-size: 0.85rem;">
									Password <span class="text-danger">*</span>
								</label>
								<input id="input-password" type="password" class="form-control" bind:value={newUser.password} required minlength="6" placeholder="Min. 6 characters" />
							</div>
							<div class="col-md-6">
								<label for="select-role" class="form-label fw-medium" style="font-size: 0.85rem;">Role</label>
								<select id="select-role" class="form-select" bind:value={newUser.role_name} style="border-radius: 10px; padding: 0.65rem 1rem; border: 1.5px solid #e0ddd8;">
									{#each availableRoles as role}
										<option value={role}>{role.charAt(0).toUpperCase() + role.slice(1)}</option>
									{/each}
								</select>
							</div>
						</div>
						<div class="d-flex justify-content-end gap-2 mt-4">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showCreate = false}>Cancel</button>
							<button id="btn-submit-user" type="submit" class="btn btn-coral" disabled={formLoading}>
								{#if formLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}
								Create User
							</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}
