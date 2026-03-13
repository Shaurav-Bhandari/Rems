<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let activeTab = $state<'categories' | 'items'>('items');
	let categories = $state<any[]>([]);
	let items = $state<any[]>([]);
	let loading = $state(true);

	// Category form
	let showCatModal = $state(false);
	let catForm = $state({ name: '', description: '' });
	let catLoading = $state(false);
	let catError = $state('');

	// Item form
	let showItemModal = $state(false);
	let editingItemId = $state<string | null>(null);
	let itemForm = $state({ name: '', description: '', price: 0, category_id: '', is_available: true });
	let itemLoading = $state(false);
	let itemError = $state('');

	onMount(async () => { await loadAll(); });

	async function loadAll() {
		loading = true;
		const [catRes, itemRes] = await Promise.all([
			api.get<any>('/menu/categories'),
			api.get<any>('/menu/items')
		]);
		if (catRes.success && catRes.data) categories = Array.isArray(catRes.data) ? catRes.data : catRes.data?.categories || [];
		if (itemRes.success && itemRes.data) items = Array.isArray(itemRes.data) ? itemRes.data : itemRes.data?.items || [];
		loading = false;
	}

	async function createCategory() {
		catLoading = true;
		catError = '';
		const res = await api.post('/menu/categories', catForm);
		if (!res.success) { catError = res.message; catLoading = false; return; }
		catLoading = false;
		showCatModal = false;
		await loadAll();
	}

	function openCreateItem() {
		itemForm = { name: '', description: '', price: 0, category_id: '', is_available: true };
		editingItemId = null;
		itemError = '';
		showItemModal = true;
	}

	function openEditItem(item: any) {
		itemForm = {
			name: item.name || '',
			description: item.description || '',
			price: item.price || 0,
			category_id: item.category_id || '',
			is_available: item.is_available ?? true
		};
		editingItemId = item.menu_item_id || item.id;
		itemError = '';
		showItemModal = true;
	}

	async function saveItem() {
		itemLoading = true;
		itemError = '';
		let res;
		if (editingItemId) {
			res = await api.put(`/menu/items/${editingItemId}`, itemForm);
		} else {
			res = await api.post('/menu/items', itemForm);
		}
		if (!res.success) { itemError = res.message; itemLoading = false; return; }
		itemLoading = false;
		showItemModal = false;
		await loadAll();
	}

	async function deleteItem(id: string) {
		if (!confirm('Delete this menu item?')) return;
		await api.del(`/menu/items/${id}`);
		await loadAll();
	}

	function formatCurrency(n: number) {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n || 0);
	}
</script>

<svelte:head><title>Menu — ReMS</title></svelte:head>

<div class="page-header">
	<h1>Menu</h1>
	<div class="d-flex gap-2">
		{#if activeTab === 'categories'}
			<button class="btn btn-coral" on:click={() => { showCatModal = true; catError = ''; catForm = { name: '', description: '' }; }}>
				<i class="bi bi-plus-circle me-1"></i> Add Category
			</button>
		{:else}
			<button class="btn btn-coral" on:click={openCreateItem}>
				<i class="bi bi-plus-circle me-1"></i> Add Item
			</button>
		{/if}
	</div>
</div>

<div class="page-content fade-in">
	<!-- Tabs -->
	<ul class="nav nav-pills mb-3" style="gap: 0.5rem;">
		<li class="nav-item">
			<button class="btn btn-sm" class:btn-prussian={activeTab === 'items'} class:btn-outline-secondary={activeTab !== 'items'}
				on:click={() => activeTab = 'items'}>
				<i class="bi bi-book me-1"></i> Menu Items
			</button>
		</li>
		<li class="nav-item">
			<button class="btn btn-sm" class:btn-prussian={activeTab === 'categories'} class:btn-outline-secondary={activeTab !== 'categories'}
				on:click={() => activeTab = 'categories'}>
				<i class="bi bi-tag me-1"></i> Categories
			</button>
		</li>
	</ul>

	{#if loading}
		<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
	{:else if activeTab === 'categories'}
		{#if categories.length === 0}
			<div class="empty-state"><i class="bi bi-tag"></i><p>No categories yet</p></div>
		{:else}
			<div class="row g-3">
				{#each categories as cat}
					<div class="col-md-4">
						<div class="card">
							<div class="card-body">
								<h6 class="fw-bold" style="color: var(--prussian-blue);">{cat.name || '—'}</h6>
								<p class="text-muted mb-0" style="font-size: 0.85rem;">{cat.description || 'No description'}</p>
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{:else}
		{#if items.length === 0}
			<div class="empty-state"><i class="bi bi-book"></i><p>No menu items yet</p></div>
		{:else}
			<div class="table-container">
				<div class="table-responsive">
					<table class="table">
						<thead>
							<tr>
								<th>Item</th>
								<th>Price</th>
								<th>Available</th>
								<th style="width: 120px;">Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each items as item}
								<tr>
									<td>
										<span class="fw-medium">{item.name || '—'}</span>
										{#if item.description}
											<br><small class="text-muted">{item.description.substring(0, 60)}</small>
										{/if}
									</td>
									<td class="fw-bold" style="color: var(--vibrant-coral);">{formatCurrency(item.price)}</td>
									<td>
										{#if item.is_available}
											<span class="badge-status active">Available</span>
										{:else}
											<span class="badge-status cancelled">Unavailable</span>
										{/if}
									</td>
									<td>
										<div class="d-flex gap-1">
											<button class="btn btn-sm btn-outline-prussian" on:click={() => openEditItem(item)}><i class="bi bi-pencil"></i></button>
											<button class="btn btn-sm btn-outline-danger" on:click={() => deleteItem(item.menu_item_id || item.id)}><i class="bi bi-trash"></i></button>
										</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}
	{/if}
</div>

<!-- Category Modal -->
{#if showCatModal}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showCatModal = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered">
			<div class="modal-content">
				<div class="modal-header"><h5 class="modal-title">Add Category</h5><button type="button" class="btn-close" on:click={() => showCatModal = false}></button></div>
				<div class="modal-body">
					{#if catError}<div class="alert alert-danger py-2">{catError}</div>{/if}
					<form on:submit|preventDefault={createCategory}>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Name</label><input type="text" class="form-control" bind:value={catForm.name} required /></div>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Description</label><textarea class="form-control" bind:value={catForm.description} rows="2"></textarea></div>
						<div class="d-flex justify-content-end gap-2">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showCatModal = false}>Cancel</button>
							<button type="submit" class="btn btn-coral" disabled={catLoading}>{#if catLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}Create</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}

<!-- Item Modal -->
{#if showItemModal}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showItemModal = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered">
			<div class="modal-content">
				<div class="modal-header"><h5 class="modal-title">{editingItemId ? 'Edit' : 'Add'} Menu Item</h5><button type="button" class="btn-close" on:click={() => showItemModal = false}></button></div>
				<div class="modal-body">
					{#if itemError}<div class="alert alert-danger py-2">{itemError}</div>{/if}
					<form on:submit|preventDefault={saveItem}>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Name</label><input type="text" class="form-control" bind:value={itemForm.name} required /></div>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Description</label><textarea class="form-control" bind:value={itemForm.description} rows="2"></textarea></div>
						<div class="row">
							<div class="col-md-6 mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Price</label><input type="number" step="0.01" class="form-control" bind:value={itemForm.price} required /></div>
							<div class="col-md-6 mb-3">
								<label class="form-label fw-medium" style="font-size: 0.85rem;">Available</label>
								<select class="form-select" style="border-radius: 10px;" bind:value={itemForm.is_available}>
									<option value={true}>Yes</option>
									<option value={false}>No</option>
								</select>
							</div>
						</div>
						<div class="d-flex justify-content-end gap-2">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showItemModal = false}>Cancel</button>
							<button type="submit" class="btn btn-coral" disabled={itemLoading}>{#if itemLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}{editingItemId ? 'Update' : 'Create'}</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}
