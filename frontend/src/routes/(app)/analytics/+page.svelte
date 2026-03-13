<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';

	let loading = $state(true);
	let revenueOverview = $state<any>(null);
	let revenueTrend = $state<any[]>([]);
	let orderVolume = $state<any>(null);
	let orderStatus = $state<any>(null);
	let topItems = $state<any[]>([]);

	onMount(async () => {
		loading = true;
		try {
			const [revOv, revTr, ordVol, ordSt, topIt] = await Promise.all([
				api.get<any>('/analytics/revenue/overview'),
				api.get<any>('/analytics/revenue/trend'),
				api.get<any>('/analytics/orders/volume'),
				api.get<any>('/analytics/orders/status'),
				api.get<any>('/analytics/menu/top-items')
			]);

			if (revOv.success) revenueOverview = revOv.data;
			if (revTr.success) revenueTrend = Array.isArray(revTr.data) ? revTr.data : revTr.data?.trend || [];
			if (ordVol.success) orderVolume = ordVol.data;
			if (ordSt.success) orderStatus = ordSt.data;
			if (topIt.success) topItems = Array.isArray(topIt.data) ? topIt.data : topIt.data?.items || [];
		} catch { /* API may not be running */ }
		loading = false;
	});

	function formatCurrency(n: number) {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n || 0);
	}

	function getBarWidth(value: number, max: number) {
		if (!max) return 0;
		return Math.min((value / max) * 100, 100);
	}
</script>

<svelte:head><title>Analytics — ReMS</title></svelte:head>

<div class="page-header">
	<h1>Analytics</h1>
	<span class="badge" style="background: var(--cream); color: var(--carbon-black); font-size: 0.8rem; padding: 0.5rem 0.8rem; border-radius: 10px;">
		<i class="bi bi-graph-up me-1"></i> Live Dashboard
	</span>
</div>

<div class="page-content fade-in">
	{#if loading}
		<div class="loading-spinner"><div class="spinner-border" role="status"></div></div>
	{:else}
		<!-- Revenue Overview -->
		<div class="row g-3 mb-4">
			<div class="col-md-4">
				<div class="kpi-card prussian">
					<div class="kpi-label">Total Revenue</div>
					<div class="kpi-value">{formatCurrency(revenueOverview?.total_revenue || 0)}</div>
					<div class="kpi-change"><i class="bi bi-currency-dollar"></i> Overall</div>
				</div>
			</div>
			<div class="col-md-4">
				<div class="kpi-card coral">
					<div class="kpi-label">Average Order Value</div>
					<div class="kpi-value">{formatCurrency(revenueOverview?.average_order_value || 0)}</div>
					<div class="kpi-change"><i class="bi bi-receipt"></i> Per order</div>
				</div>
			</div>
			<div class="col-md-4">
				<div class="kpi-card grape">
					<div class="kpi-label">Order Volume</div>
					<div class="kpi-value">{orderVolume?.total_orders || orderVolume?.count || 0}</div>
					<div class="kpi-change"><i class="bi bi-bag"></i> Total orders</div>
				</div>
			</div>
		</div>

		<div class="row g-3 mb-4">
			<!-- Revenue Trend (bar chart) -->
			<div class="col-lg-8">
				<div class="card">
					<div class="card-body">
						<h6 class="fw-bold mb-3" style="color: var(--prussian-blue);">
							<i class="bi bi-bar-chart me-1"></i> Revenue Trend
						</h6>
						{#if revenueTrend.length > 0}
							<div class="d-flex flex-column gap-2">
								{#each revenueTrend.slice(-10) as point}
									{@const maxRev = Math.max(...revenueTrend.map(p => p.revenue || p.amount || 0))}
									<div class="d-flex align-items-center gap-2">
										<span class="text-muted" style="font-size: 0.75rem; width: 70px; text-align: right;">
											{point.period || point.date || '—'}
										</span>
										<div class="flex-grow-1" style="height: 24px; background: #f0ece5; border-radius: 6px; overflow: hidden;">
											<div style="width: {getBarWidth(point.revenue || point.amount || 0, maxRev)}%; height: 100%; background: linear-gradient(90deg, var(--prussian-blue), var(--vibrant-coral)); border-radius: 6px; transition: width 0.5s ease;"></div>
										</div>
										<span class="fw-medium" style="font-size: 0.8rem; width: 80px;">
											{formatCurrency(point.revenue || point.amount || 0)}
										</span>
									</div>
								{/each}
							</div>
						{:else}
							<div class="empty-state"><i class="bi bi-bar-chart"></i><p>No trend data available</p></div>
						{/if}
					</div>
				</div>
			</div>

			<!-- Order Status Distribution -->
			<div class="col-lg-4">
				<div class="card">
					<div class="card-body">
						<h6 class="fw-bold mb-3" style="color: var(--prussian-blue);">
							<i class="bi bi-pie-chart me-1"></i> Order Status
						</h6>
						{#if orderStatus}
							{@const statuses = Object.entries(orderStatus).filter(([k]) => k !== 'total')}
							<div class="d-flex flex-column gap-2">
								{#each statuses as [status, count]}
									{@const total = statuses.reduce((a, [, c]) => a + (Number(c) || 0), 0)}
									<div>
										<div class="d-flex justify-content-between mb-1" style="font-size: 0.82rem;">
											<span class="text-capitalize">{status.replace(/_/g, ' ')}</span>
											<span class="fw-bold">{count}</span>
										</div>
										<div style="height: 6px; background: #f0ece5; border-radius: 3px; overflow: hidden;">
											<div style="width: {getBarWidth(Number(count), total)}%; height: 100%; background: var(--vibrant-coral); border-radius: 3px;"></div>
										</div>
									</div>
								{/each}
							</div>
						{:else}
							<div class="empty-state"><i class="bi bi-pie-chart"></i><p>No status data</p></div>
						{/if}
					</div>
				</div>
			</div>
		</div>

		<!-- Top Selling Items -->
		<div class="card">
			<div class="card-body">
				<h6 class="fw-bold mb-3" style="color: var(--prussian-blue);">
					<i class="bi bi-trophy me-1"></i> Top Selling Items
				</h6>
				{#if topItems.length > 0}
					<div class="table-responsive">
						<table class="table mb-0">
							<thead>
								<tr>
									<th style="width: 40px;">#</th>
									<th>Item</th>
									<th>Orders</th>
									<th>Revenue</th>
								</tr>
							</thead>
							<tbody>
								{#each topItems.slice(0, 10) as item, i}
									<tr>
										<td>
											{#if i < 3}
												<span style="color: {['#FFD700', '#C0C0C0', '#CD7F32'][i]}; font-size: 1.1rem;">
													<i class="bi bi-trophy-fill"></i>
												</span>
											{:else}
												<span class="text-muted">{i + 1}</span>
											{/if}
										</td>
										<td class="fw-medium">{item.name || item.item_name || '—'}</td>
										<td>{item.order_count || item.orders || 0}</td>
										<td class="fw-bold" style="color: var(--vibrant-coral);">
											{formatCurrency(item.total_revenue || item.revenue || 0)}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{:else}
					<div class="empty-state"><i class="bi bi-trophy"></i><p>No data available yet</p></div>
				{/if}
			</div>
		</div>
	{/if}
</div>
