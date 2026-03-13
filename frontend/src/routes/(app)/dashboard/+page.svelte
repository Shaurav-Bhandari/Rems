<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	let stats = $state({
		totalRevenue: 0,
		totalOrders: 0,
		activeItems: 0,
		lowStock: 0
	});
	let loading = $state(true);
	let recentOrders = $state<any[]>([]);

	onMount(async () => {
		loading = true;
		try {
			const [revenueRes, ordersRes] = await Promise.all([
				api.get<any>('/analytics/revenue/overview'),
				api.get<any>('/orders')
			]);

			if (revenueRes.success && revenueRes.data) {
				stats.totalRevenue = revenueRes.data.total_revenue || 0;
			}
			if (ordersRes.success && ordersRes.data) {
				const orders = Array.isArray(ordersRes.data) ? ordersRes.data : ordersRes.data?.orders || [];
				stats.totalOrders = orders.length;
				recentOrders = orders.slice(0, 5);
			}
		} catch {
			// API may not be running yet
		}
		loading = false;
	});

	function formatCurrency(amount: number) {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amount);
	}
</script>

<svelte:head>
	<title>Dashboard — ReMS</title>
</svelte:head>

<div class="page-header">
	<div>
		<h1>Dashboard</h1>
		<p class="text-muted mb-0" style="font-size: 0.85rem;">
			Welcome back{$currentUser ? `, ${$currentUser.full_name}` : ''}
		</p>
	</div>
	<div class="d-flex gap-2 align-items-center">
		<span class="badge" style="background: var(--cream); color: var(--carbon-black); font-size: 0.8rem; padding: 0.5rem 0.8rem; border-radius: 10px;">
			<i class="bi bi-calendar3 me-1"></i>
			{new Date().toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' })}
		</span>
	</div>
</div>

<div class="page-content fade-in">
	{#if loading}
		<div class="loading-spinner">
			<div class="spinner-border" role="status">
				<span class="visually-hidden">Loading...</span>
			</div>
		</div>
	{:else}
		<!-- KPI Cards -->
		<div class="row g-3 mb-4">
			<div class="col-md-6 col-xl-3">
				<div class="kpi-card grape">
					<div class="kpi-label">Total Revenue</div>
					<div class="kpi-value">{formatCurrency(stats.totalRevenue)}</div>
					<div class="kpi-change">
						<i class="bi bi-graph-up-arrow me-1"></i> This period
					</div>
				</div>
			</div>

			<div class="col-md-6 col-xl-3">
				<div class="kpi-card coral">
					<div class="kpi-label">Total Orders</div>
					<div class="kpi-value">{stats.totalOrders}</div>
					<div class="kpi-change">
						<i class="bi bi-bag-check me-1"></i> All time
					</div>
				</div>
			</div>

			<div class="col-md-6 col-xl-3">
				<div class="kpi-card prussian">
					<div class="kpi-label">Menu Items</div>
					<div class="kpi-value">{stats.activeItems}</div>
					<div class="kpi-change">
						<i class="bi bi-book me-1"></i> Active items
					</div>
				</div>
			</div>

			<div class="col-md-6 col-xl-3">
				<div class="kpi-card green">
					<div class="kpi-label">Low Stock Alerts</div>
					<div class="kpi-value">{stats.lowStock}</div>
					<div class="kpi-change">
						<i class="bi bi-exclamation-triangle me-1"></i> Needs attention
					</div>
				</div>
			</div>
		</div>

		<div class="row g-3">
			<!-- Recent Orders -->
			<div class="col-lg-8">
				<div class="card">
					<div class="card-body p-0">
						<div class="d-flex align-items-center justify-content-between p-3 pb-0">
							<h6 class="fw-bold mb-0" style="color: var(--prussian-blue);">Recent Orders</h6>
							<a href="/orders" class="btn btn-sm btn-outline-prussian">View All</a>
						</div>
						{#if recentOrders.length > 0}
							<div class="table-responsive">
								<table class="table mb-0">
									<thead>
										<tr>
											<th>Order ID</th>
											<th>Status</th>
											<th>Total</th>
											<th>Date</th>
										</tr>
									</thead>
									<tbody>
										{#each recentOrders as order}
											<tr>
												<td class="fw-medium">#{order.order_id?.substring(0, 8) || '—'}</td>
												<td>
													<span class="badge-status {order.status?.toLowerCase() || 'pending'}">
														{order.status || 'Pending'}
													</span>
												</td>
												<td>{formatCurrency(order.total_amount || 0)}</td>
												<td class="text-muted" style="font-size: 0.85rem;">
													{order.created_at ? new Date(order.created_at).toLocaleDateString() : '—'}
												</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{:else}
							<div class="empty-state">
								<i class="bi bi-receipt"></i>
								<p>No orders yet</p>
							</div>
						{/if}
					</div>
				</div>
			</div>

			<!-- Quick Actions -->
			<div class="col-lg-4">
				<div class="card">
					<div class="card-body">
						<h6 class="fw-bold mb-3" style="color: var(--prussian-blue);">Quick Actions</h6>
						<div class="d-grid gap-2">
							<a href="/orders" class="btn btn-coral text-start">
								<i class="bi bi-plus-circle me-2"></i> New Order
							</a>
							<a href="/menu" class="btn btn-prussian text-start">
								<i class="bi bi-plus-circle me-2"></i> Add Menu Item
							</a>
							<a href="/inventory" class="btn btn-outline-prussian text-start">
								<i class="bi bi-box-seam me-2"></i> Stock Check
							</a>
							<a href="/analytics" class="btn btn-outline-secondary text-start">
								<i class="bi bi-graph-up me-2"></i> View Reports
							</a>
						</div>
					</div>
				</div>

				<!-- Tenant Info -->
				{#if $currentUser}
					<div class="card mt-3">
						<div class="card-body">
							<h6 class="fw-bold mb-3" style="color: var(--prussian-blue);">Your Profile</h6>
							<div class="d-flex flex-column gap-2" style="font-size: 0.88rem;">
								<div class="d-flex justify-content-between">
									<span class="text-muted">Name</span>
									<span class="fw-medium">{$currentUser.full_name}</span>
								</div>
								<div class="d-flex justify-content-between">
									<span class="text-muted">Role</span>
									<span class="badge" style="background: rgba(193, 238, 255, 0.2); color: var(--prussian-blue);">
										{$currentUser.default_role}
									</span>
								</div>
								<div class="d-flex justify-content-between">
									<span class="text-muted">Organization</span>
									<span class="fw-medium">{$currentUser.tenant_name}</span>
								</div>
							</div>
						</div>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
