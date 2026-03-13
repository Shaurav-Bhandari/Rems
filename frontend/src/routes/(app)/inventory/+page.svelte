<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let items = $state<any[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let showAdjust = $state(false);
	let editingId = $state<string | null>(null);
	let adjustingItem = $state<any>(null);

	let form = $state({ name: '', sku: '', quantity: 0, unit: '', min_stock: 0, cost_per_unit: 0 });
	let adjustForm = $state({ quantity: 0, reason: '' });
	let formError = $state('');
	let formLoading = $state(false);

	onMount(async () => { await loadItems(); });

	async function loadItems() {
		loading = true;
		const res = await api.get<any>('/inventory');
		if (res.success && res.data) {
			items = Array.isArray(res.data) ? res.data : res.data?.items || [];
		}
		loading = false;
	}

	function openCreate() {
		form = { name: '', sku: '', quantity: 0, unit: 'pcs', min_stock: 0, cost_per_unit: 0 };
		editingId = null;
		formError = '';
		showModal = true;
	}

	function openEdit(item: any) {
		form = {
			name: item.name || '',
			sku: item.sku || '',
			quantity: item.quantity || 0,
			unit: item.unit || 'pcs',
			min_stock: item.minimum_stock_level || item.min_stock || 0,
			cost_per_unit: item.cost_per_unit || 0
		};
		editingId = item.inventory_id || item.id;
		formError = '';
		showModal = true;
	}

	function openAdjust(item: any) {
		adjustingItem = item;
		adjustForm = { quantity: 0, reason: '' };
		formError = '';
		showAdjust = true;
	}

	async function handleSubmit() {
		formLoading = true;
		formError = '';
		let res;
		if (editingId) {
			res = await api.put(`/inventory/${editingId}`, form);
		} else {
			res = await api.post('/inventory', form);
		}
		if (!res.success) { formError = res.message; formLoading = false; return; }
		formLoading = false;
		showModal = false;
		await loadItems();
	}

	async function handleAdjust() {
		formLoading = true;
		formError = '';
		const id = adjustingItem?.inventory_id || adjustingItem?.id;
		const res = await api.post(`/inventory/${id}/adjust`, adjustForm);
		if (!res.success) { formError = res.message; formLoading = false; return; }
		formLoading = false;
		showAdjust = false;
		await loadItems();
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this inventory item?')) return;
		await api.del(`/inventory/${id}`);
		await loadItems();
	}

	function isLowStock(item: any) {
		return (item.quantity || 0) <= (item.minimum_stock_level || item.min_stock || 0);
	}
</script>

<svelte:head><title>Inventory — ReMS</title></svelte:head>

<div class="page-header">
	<h1>Inventory</h1>
	<button class="btn btn-coral" on:click={openCreate}>
		<i class="bi bi-plus-circle me-1"></i> Add Item
	</button>
</div>

<div class="page-content fade-in">
	{#if loading}
		<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
	{:else if items.length === 0}
		<div class="empty-state"><i class="bi bi-box-seam"></i><p>No inventory items yet</p></div>
	{:else}
		<div class="table-container">
			<div class="table-responsive">
				<table class="table">
					<thead>
						<tr>
							<th>Item</th>
							<th>SKU</th>
							<th>Quantity</th>
							<th>Unit</th>
							<th>Status</th>
							<th style="width: 160px;">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each items as item}
							<tr>
								<td class="fw-medium">{item.name || '—'}</td>
								<td class="text-muted" style="font-size: 0.85rem;">{item.sku || '—'}</td>
								<td class="fw-bold">{item.quantity ?? 0}</td>
								<td>{item.unit || '—'}</td>
								<td>
									{#if isLowStock(item)}
										<span class="badge-status cancelled">
											<i class="bi bi-exclamation-triangle me-1"></i>Low Stock
										</span>
									{:else}
										<span class="badge-status active">In Stock</span>
									{/if}
								</td>
								<td>
									<div class="d-flex gap-1">
										<button class="btn btn-sm btn-outline-prussian" title="Adjust stock" on:click={() => openAdjust(item)}>
											<i class="bi bi-arrow-down-up"></i>
										</button>
										<button class="btn btn-sm btn-outline-prussian" on:click={() => openEdit(item)}>
											<i class="bi bi-pencil"></i>
										</button>
										<button class="btn btn-sm btn-outline-danger" on:click={() => handleDelete(item.inventory_id || item.id)}>
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

<!-- Create/Edit Modal -->
{#if showModal}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showModal = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered">
			<div class="modal-content">
				<div class="modal-header"><h5 class="modal-title">{editingId ? 'Edit' : 'Add'} Inventory Item</h5><button type="button" class="btn-close" on:click={() => showModal = false}></button></div>
				<div class="modal-body">
					{#if formError}<div class="alert alert-danger py-2">{formError}</div>{/if}
					<form on:submit|preventDefault={handleSubmit}>
						<div class="row">
							<div class="col-md-8 mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Name</label><input type="text" class="form-control" bind:value={form.name} required /></div>
							<div class="col-md-4 mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">SKU</label><input type="text" class="form-control" bind:value={form.sku} /></div>
						</div>
						<div class="row">
							<div class="col-md-4 mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Quantity</label><input type="number" class="form-control" bind:value={form.quantity} /></div>
							<div class="col-md-4 mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Unit</label><input type="text" class="form-control" bind:value={form.unit} placeholder="pcs, kg, etc." /></div>
							<div class="col-md-4 mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Min Stock</label><input type="number" class="form-control" bind:value={form.min_stock} /></div>
						</div>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Cost per Unit</label><input type="number" step="0.01" class="form-control" bind:value={form.cost_per_unit} /></div>
						<div class="d-flex justify-content-end gap-2">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showModal = false}>Cancel</button>
							<button type="submit" class="btn btn-coral" disabled={formLoading}>{#if formLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}{editingId ? 'Update' : 'Create'}</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}

<!-- Adjust Stock Modal -->
{#if showAdjust}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showAdjust = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered modal-sm">
			<div class="modal-content">
				<div class="modal-header"><h5 class="modal-title">Adjust Stock</h5><button type="button" class="btn-close" on:click={() => showAdjust = false}></button></div>
				<div class="modal-body">
					<p class="mb-2" style="font-size: 0.85rem;">
						<strong>{adjustingItem?.name}</strong> — Current: <strong>{adjustingItem?.quantity ?? 0}</strong>
					</p>
					{#if formError}<div class="alert alert-danger py-2">{formError}</div>{/if}
					<form on:submit|preventDefault={handleAdjust}>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Adjustment (+ or −)</label><input type="number" class="form-control" bind:value={adjustForm.quantity} required /></div>
						<div class="mb-3"><label class="form-label fw-medium" style="font-size: 0.85rem;">Reason</label><input type="text" class="form-control" bind:value={adjustForm.reason} placeholder="Restock, damage, etc." /></div>
						<div class="d-flex justify-content-end gap-2">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showAdjust = false}>Cancel</button>
							<button type="submit" class="btn btn-coral" disabled={formLoading}>{#if formLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}Adjust</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}
