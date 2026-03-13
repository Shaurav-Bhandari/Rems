<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let orders = $state<any[]>([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let formError = $state('');
	let formLoading = $state(false);
	let statusFilter = $state('all');

	let newOrder = $state({ restaurant_id: '', notes: '' });

	const statusColors: Record<string, string> = {
		pending: 'pending',
		confirmed: 'active',
		preparing: 'active',
		ready: 'completed',
		delivered: 'completed',
		completed: 'completed',
		cancelled: 'cancelled'
	};

	const statusFlow: Record<string, string[]> = {
		pending: ['confirmed', 'cancelled'],
		confirmed: ['preparing', 'cancelled'],
		preparing: ['ready'],
		ready: ['delivered'],
		delivered: ['completed'],
	};

	onMount(async () => { await loadOrders(); });

	async function loadOrders() {
		loading = true;
		const res = await api.get<any>('/orders');
		if (res.success && res.data) {
			orders = Array.isArray(res.data) ? res.data : res.data?.orders || [];
		}
		loading = false;
	}

	async function updateStatus(orderId: string, newStatus: string) {
		const res = await api.put(`/orders/${orderId}/status`, { status: newStatus });
		if (res.success) await loadOrders();
	}

	async function handleCreate() {
		formLoading = true;
		formError = '';
		const res = await api.post('/orders', newOrder);
		if (!res.success) {
			formError = res.message || 'Failed to create order';
			formLoading = false;
			return;
		}
		formLoading = false;
		showCreate = false;
		await loadOrders();
	}

	async function handleDelete(id: string) {
		if (!confirm('Cancel this order?')) return;
		await api.del(`/orders/${id}`);
		await loadOrders();
	}

	$effect(() => {
		// filtered orders reactivity
	});

	function getFilteredOrders() {
		if (statusFilter === 'all') return orders;
		return orders.filter(o => o.status?.toLowerCase() === statusFilter);
	}

	function formatCurrency(n: number) {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n || 0);
	}
</script>

<svelte:head><title>Orders — ReMS</title></svelte:head>

<div class="page-header">
	<h1>Orders</h1>
	<button class="btn btn-coral" on:click={() => { showCreate = true; formError = ''; }}>
		<i class="bi bi-plus-circle me-1"></i> New Order
	</button>
</div>

<div class="page-content fade-in">
	<!-- Filters -->
	<div class="d-flex gap-2 mb-3 flex-wrap">
		{#each ['all', 'pending', 'confirmed', 'preparing', 'ready', 'delivered', 'completed', 'cancelled'] as st}
			<button class="btn btn-sm" class:btn-prussian={statusFilter === st} class:btn-outline-secondary={statusFilter !== st}
				on:click={() => statusFilter = st}>
				{st.charAt(0).toUpperCase() + st.slice(1)}
			</button>
		{/each}
	</div>

	{#if loading}
		<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
	{:else if getFilteredOrders().length === 0}
		<div class="empty-state">
			<i class="bi bi-receipt"></i>
			<p>No orders found</p>
		</div>
	{:else}
		<div class="row g-3">
			{#each getFilteredOrders() as order}
				<div class="col-md-6 col-xl-4">
					<div class="card">
						<div class="card-body">
							<div class="d-flex justify-content-between align-items-start mb-2">
								<h6 class="fw-bold mb-0" style="color: var(--prussian-blue);">
									#{order.order_id?.substring(0, 8) || '—'}
								</h6>
								<span class="badge-status {statusColors[order.status?.toLowerCase()] || 'pending'}">
									{order.status || 'Pending'}
								</span>
							</div>

							<div class="d-flex justify-content-between mb-2" style="font-size: 0.88rem;">
								<span class="text-muted">Total</span>
								<span class="fw-bold" style="color: var(--vibrant-coral);">{formatCurrency(order.total_amount)}</span>
							</div>

							{#if order.notes}
								<p class="text-muted mb-2" style="font-size: 0.82rem;"><i class="bi bi-chat-text me-1"></i>{order.notes}</p>
							{/if}

							<div class="text-muted mb-3" style="font-size: 0.78rem;">
								<i class="bi bi-clock me-1"></i>
								{order.created_at ? new Date(order.created_at).toLocaleString() : '—'}
							</div>

							<!-- Status actions -->
							<div class="d-flex gap-1 flex-wrap">
								{#each statusFlow[order.status?.toLowerCase()] || [] as next}
									<button class="btn btn-sm"
										class:btn-coral={next === 'cancelled'}
										class:btn-prussian={next !== 'cancelled'}
										on:click={() => updateStatus(order.order_id || order.id, next)}>
										{next.charAt(0).toUpperCase() + next.slice(1)}
									</button>
								{/each}
								{#if order.status?.toLowerCase() === 'pending'}
									<button class="btn btn-sm btn-outline-danger" on:click={() => handleDelete(order.order_id || order.id)}>
										<i class="bi bi-trash"></i>
									</button>
								{/if}
							</div>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- Create Order Modal -->
{#if showCreate}
	<div class="modal d-block" style="background: rgba(0,0,0,0.5);" on:click|self={() => showCreate = false} role="dialog">
		<div class="modal-dialog modal-dialog-centered">
			<div class="modal-content">
				<div class="modal-header">
					<h5 class="modal-title">New Order</h5>
					<button type="button" class="btn-close" on:click={() => showCreate = false}></button>
				</div>
				<div class="modal-body">
					{#if formError}<div class="alert alert-danger py-2">{formError}</div>{/if}
					<form on:submit|preventDefault={handleCreate}>
						<div class="mb-3">
							<label class="form-label fw-medium" style="font-size: 0.85rem;">Notes</label>
							<textarea class="form-control" bind:value={newOrder.notes} rows="3" placeholder="Order notes..."></textarea>
						</div>
						<div class="d-flex justify-content-end gap-2">
							<button type="button" class="btn btn-outline-secondary" on:click={() => showCreate = false}>Cancel</button>
							<button type="submit" class="btn btn-coral" disabled={formLoading}>
								{#if formLoading}<span class="spinner-border spinner-border-sm me-1"></span>{/if}
								Create Order
							</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
{/if}
