<script lang="ts">
  import { onMount } from 'svelte';
  import { getOrders, createOrder, updateOrderStatus, deleteOrder } from '$lib/api';
  import type { Order, OrderItem } from '$lib/types';

  let orders: Order[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let userId = $state('');
  let items: OrderItem[] = $state([{ product_id: 0, product_name: '', quantity: 1, unit_price: 0 }]);
  let creating = $state(false);

  const statuses = ['pending','processing','shipped','delivered','cancelled'];

  function addItem() {
    items = [...items, { product_id: 0, product_name: '', quantity: 1, unit_price: 0 }];
  }
  function removeItem(i: number) {
    items = items.filter((_, idx) => idx !== i);
  }

  async function load() {
    loading = true; error = '';
    try { orders = await getOrders(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
    finally { loading = false; }
  }

  async function handleCreate() {
    const uid = parseInt(userId);
    if (!uid || items.length === 0) return;
    creating = true; error = '';
    try {
      await createOrder(uid, items);
      userId = '';
      items = [{ product_id: 0, product_name: '', quantity: 1, unit_price: 0 }];
      await load();
    } catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed to create order'; }
    finally { creating = false; }
  }

  async function handleStatus(id: number, status: string) {
    try { await updateOrderStatus(id, status); await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
  }

  async function handleDelete(id: number) {
    try { await deleteOrder(id); await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
  }

  onMount(load);
</script>

<div class="page-header">
  <div>
    <div class="page-title">Orders</div>
    <div class="page-sub">{orders.length} total</div>
  </div>
  <button class="btn-ghost" onclick={load}>↻ Refresh</button>
</div>

{#if error}<div class="alert-error">{error}</div>{/if}

<div class="card">
  <div class="card-body">
    <div class="section-label">New Order</div>
    <div class="form-row" style="margin-bottom: 16px;">
      <div class="form-field" style="max-width: 130px;">
        <label class="field-label">User ID</label>
        <input bind:value={userId} type="number" placeholder="1" />
      </div>
    </div>

    <div class="items-header">
      <span class="field-label">Items</span>
      <button class="btn-ghost btn-sm" onclick={addItem}>+ Add item</button>
    </div>

    <div class="items-list">
      {#each items as item, i}
        <div class="item-row">
          <div class="form-field" style="width: 90px;">
            {#if i === 0}<label class="field-label">Product ID</label>{/if}
            <input bind:value={item.product_id} type="number" placeholder="ID" />
          </div>
          <div class="form-field" style="flex: 1; min-width: 140px;">
            {#if i === 0}<label class="field-label">Product name</label>{/if}
            <input bind:value={item.product_name} placeholder="e.g. T-Shirt" />
          </div>
          <div class="form-field" style="width: 75px;">
            {#if i === 0}<label class="field-label">Qty</label>{/if}
            <input bind:value={item.quantity} type="number" min="1" />
          </div>
          <div class="form-field" style="width: 110px;">
            {#if i === 0}<label class="field-label">Unit price</label>{/if}
            <input bind:value={item.unit_price} type="number" step="0.01" placeholder="0.00" />
          </div>
          {#if items.length > 1}
            <button
              class="btn-danger btn-sm remove-btn"
              style={i === 0 ? 'margin-top: 20px' : ''}
              onclick={() => removeItem(i)}
            >×</button>
          {/if}
        </div>
      {/each}
    </div>

    <button class="btn-primary" onclick={handleCreate} disabled={creating} style="margin-top: 16px;">
      {creating ? 'Creating…' : '+ Create order'}
    </button>
  </div>
</div>

<div class="card">
  {#if loading}
    <div class="empty-state"><span class="icon">⏳</span>Loading…</div>
  {:else if orders.length === 0}
    <div class="empty-state"><span class="icon">◈</span>No orders yet</div>
  {:else}
    <table>
      <thead>
        <tr><th>ID</th><th>User</th><th>Items</th><th>Total</th><th>Status</th><th>Created</th><th></th></tr>
      </thead>
      <tbody>
        {#each orders as o}
          <tr>
            <td><span class="id-badge">#{o.id}</span></td>
            <td><span class="id-badge">user #{o.user_id}</span></td>
            <td>
              <div class="item-chips">
                {#each o.items as item}
                  <span class="item-chip">{item.product_name} ×{item.quantity}</span>
                {/each}
              </div>
            </td>
            <td class="amount">฿{o.total_amount.toLocaleString('th-TH', { minimumFractionDigits: 2 })}</td>
            <td>
              <select
                class="status-select badge badge-{o.status}"
                value={o.status}
                onchange={(e) => handleStatus(o.id, (e.target as HTMLSelectElement).value)}
              >
                {#each statuses as s}
                  <option value={s}>{s}</option>
                {/each}
              </select>
            </td>
            <td class="text-muted">{new Date(o.created_at).toLocaleDateString('en-GB', { day:'2-digit', month:'short' })}</td>
            <td><button class="btn-danger btn-sm" onclick={() => handleDelete(o.id)}>Delete</button></td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .section-label { font-size: 13px; font-weight: 600; color: #334155; margin-bottom: 12px; }
  .items-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
  .items-list { display: flex; flex-direction: column; gap: 8px; }
  .item-row { display: flex; gap: 10px; align-items: flex-end; }
  .remove-btn { flex-shrink: 0; }

  .item-chips { display: flex; flex-wrap: wrap; gap: 4px; }
  .item-chip { background: #f1f5f9; color: #475569; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
  .id-badge { background: #f1f5f9; color: #64748b; padding: 2px 8px; border-radius: 5px; font-size: 12px; font-family: monospace; }
  .amount { font-weight: 600; color: #0f172a; font-variant-numeric: tabular-nums; }

  .status-select {
    border: none;
    cursor: pointer;
    font-size: 11px;
    font-weight: 600;
    width: auto;
    padding: 3px 9px;
  }
</style>
