<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let restaurants = $state<any[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingId = $state<string | null>(null);

	let form = $state({ name: '', address: '', phone: '', email: '', cuisine_type: '' });
	let formError = $state('');
	let formLoading = $state(false);

	onMount(async () => {
		await loadRestaurants();
	});

	async function loadRestaurants() {
		loading = true;
		const res = await api.get<any>('/restaurants');
		if (res.success && res.data) {
			restaurants = Array.isArray(res.data) ? res.data : res.data?.restaurants || [];
		}
		loading = false;
	}

	function openCreate() {
		form = { name: '', address: '', phone: '', email: '', cuisine_type: '' };
		editingId = null;
		formError = '';
		showModal = true;
	}

	function openEdit(r: any) {
		form = {
			name: r.name || r.restaurant_name || '',
			address: r.address || '',
			phone: r.phone || '',
			email: r.email || '',
			cuisine_type: r.cuisine_type || ''
		};
		editingId = r.restaurant_id || r.id;
		formError = '';
		showModal = true;
	}

	async function handleSubmit() {
		formLoading = true;
		formError = '';

		const payload = { ...form };
		let res;

		if (editingId) {
			res = await api.put(`/restaurants/${editingId}`, payload);
		} else {
			res = await api.post('/restaurants', payload);
		}

		if (!res.success) {
			formError = res.message || 'Operation failed';
			formLoading = false;
			return;
		}

		formLoading = false;
		showModal = false;
		await loadRestaurants();
	}

	async function handleDelete(id: string) {
		if (!confirm('Are you sure you want to delete this restaurant?')) return;
		await api.del(`/restaurants/${id}`);
		await loadRestaurants();
	}
</script>

<svelte:head><title>Restaurants — ReMS</title></svelte:head>

<div class="page-header">
	<h1>Restaurants</h1>
	<button class="btn btn-coral" on:click={openCreate}>
		<i class="bi bi-plus-circle me-1"></i> Add Restaurant
	</button>
</div>

<div class="page-content fade-in">
	{#if loading}
		<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
	{:else if restaurants.length === 0}
		<div class="empty-state">
			<i class="bi bi-shop"></i>
			<p>No restaurants yet. Create your first one!</p>
		</div>
	{:else}
		<div class="table-container">
			<div class="table-responsive">
				<table class="table">
					<thead>
						<tr>
							<th>Name</th>
							<th>Cuisine</th>
							<th>Address</th>
							<th>Phone</th>
							<th style="width: 120px;">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each restaurants as r}
							<tr>
								<td class="fw-medium">{r.name || r.restaurant_name || '—'}</td>
								<td><span class="badge-status active">{r.cuisine_type || '—'}</span></td>
								<td class="text-muted">{r.address || '—'}</td>
								<td>{r.phone || '—'}</td>
								<td>
									<div class="d-flex gap-1">
										<button class="btn btn-sm btn-outline-prussian" on:click={() => openEdit(r)}>
											<i class="bi bi-pencil"></i>
										</button>
										<button class="btn btn-sm btn-outline-danger" on:click={() => handleDelete(r.restaurant_id || r.id)}>
											<i class="bi bi-trash"></i>
										</button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>

<!-- Modal -->
{#if showModal}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showModal = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered">
			<div class="modal-content">
				<div class="modal-header">
					<h5 class="modal-title">{editingId ? 'Edit' : 'Add'} Restaurant</h5>
					<button type="button" class="btn-close" on:click={() => showModal = false}></button>
				</div>
				<div class="modal-body">
					{#if formError}
						<div class="alert alert-danger py-2">{formError}</div>
					{/if}
					<form on:submit|preventDefault={handleSubmit}>
						<div class="mb-3">
							<label class="form-label fw-medium" style="font-size: 0.85rem;">Restaurant Name</label>
							<input type="text" class="form-control" bind:value={form.name} required />
						</div>
						<div class="mb-3">
							<label class="form-label fw-medium" style="font-size: 0.85rem;">Cuisine Type</label>
							<input type="text" class="form-control" bind:value={form.cuisine_type} placeholder="Italian, Thai, etc." />
						</div>
						<div class="mb-3">
							<label class="form-label fw-medium" style="font-size: 0.85rem;">Address</label>
							<input type="text" class="form-control" bind:value={form.address} />
						</div>
						<div class="row">
							<div class="col-md-6 mb-3">
								<label class="form-label fw-medium" style="font-size: 0.85rem;">Phone</label>
								<input type="tel" class="form-control" bind:value={form.phone} />
							</div>
							<div class="col-md-6 mb-3">
								<label class="form-label fw-medium" style="font-size: 0.85rem;">Email</label>
								<input type="email" class="form-control" bind:value={form.email} />
							</div>
						</div>
						<div class="d-flex justify-content-end gap-2">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showModal = false}>Cancel</button>
							<button type="submit" class="btn btn-coral" disabled={formLoading}>
								{#if formLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}
								{editingId ? 'Update' : 'Create'}
							</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}
