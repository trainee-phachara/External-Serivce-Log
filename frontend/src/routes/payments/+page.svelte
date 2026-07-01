<script lang="ts">
  import { onMount } from 'svelte';
  import { getPayments, createPayment, updatePaymentStatus, deletePayment } from '$lib/api';
  import type { Payment } from '$lib/types';

  let payments: Payment[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let orderId = $state('');
  let userId = $state('');
  let amount = $state('');
  let method = $state('promptpay');
  let creating = $state(false);

  const methods = ['credit_card', 'bank_transfer', 'promptpay'];
  const statuses = ['pending', 'completed', 'failed', 'refunded'];

  const methodIcon: Record<string, string> = {
    credit_card: '💳',
    bank_transfer: '🏦',
    promptpay: '📱',
  };

  async function load() {
    loading = true; error = '';
    try { payments = await getPayments(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
    finally { loading = false; }
  }

  async function handleCreate() {
    const oid = parseInt(orderId), uid = parseInt(userId), amt = parseFloat(amount);
    if (!oid || !uid || !amt || amt <= 0) return;
    creating = true; error = '';
    try {
      await createPayment(oid, uid, amt, method);
      orderId = ''; userId = ''; amount = ''; method = 'promptpay';
      await load();
    } catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
    finally { creating = false; }
  }

  async function handleStatus(id: number, status: string) {
    try { await updatePaymentStatus(id, status); await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
  }

  async function handleDelete(id: number) {
    try { await deletePayment(id); await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
  }

  const totalAmount = $derived(
    payments.reduce((sum, p) => sum + p.amount, 0)
  );

  onMount(load);
</script>

<div class="page-header">
  <div>
    <div class="page-title">Payments</div>
    <div class="page-sub">{payments.length} transactions · ฿{totalAmount.toLocaleString('th-TH', { minimumFractionDigits: 2 })} total</div>
  </div>
  <button class="btn-ghost" onclick={load}>↻ Refresh</button>
</div>

{#if error}<div class="alert-error">{error}</div>{/if}

<div class="card">
  <div class="card-body">
    <div class="section-label">New Payment</div>
    <div class="form-row">
      <div class="form-field" style="width: 120px;">
        <label class="field-label">Order ID</label>
        <input bind:value={orderId} type="number" placeholder="1" />
      </div>
      <div class="form-field" style="width: 120px;">
        <label class="field-label">User ID</label>
        <input bind:value={userId} type="number" placeholder="1" />
      </div>
      <div class="form-field" style="width: 140px;">
        <label class="field-label">Amount (฿)</label>
        <input bind:value={amount} type="number" step="0.01" placeholder="0.00" />
      </div>
      <div class="form-field" style="width: 170px;">
        <label class="field-label">Method</label>
        <select bind:value={method}>
          {#each methods as m}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </div>
      <div class="form-field" style="align-self: flex-end;">
        <button class="btn-primary" onclick={handleCreate} disabled={creating}>
          {creating ? 'Creating…' : '+ Create payment'}
        </button>
      </div>
    </div>
  </div>
</div>

<div class="card">
  {#if loading}
    <div class="empty-state"><span class="icon">⏳</span>Loading…</div>
  {:else if payments.length === 0}
    <div class="empty-state"><span class="icon">◆</span>No payments yet</div>
  {:else}
    <table>
      <thead>
        <tr><th>ID</th><th>Order</th><th>User</th><th>Amount</th><th>Method</th><th>Status</th><th>Created</th><th></th></tr>
      </thead>
      <tbody>
        {#each payments as p}
          <tr>
            <td><span class="id-badge">#{p.id}</span></td>
            <td><span class="id-badge">order #{p.order_id}</span></td>
            <td><span class="id-badge">user #{p.user_id}</span></td>
            <td class="amount">฿{p.amount.toLocaleString('th-TH', { minimumFractionDigits: 2 })}</td>
            <td>
              <span class="method-badge">
                {methodIcon[p.method] ?? ''} {p.method.replace('_', ' ')}
              </span>
            </td>
            <td>
              <select
                class="status-select badge badge-{p.status}"
                value={p.status}
                onchange={(e) => handleStatus(p.id, (e.target as HTMLSelectElement).value)}
              >
                {#each statuses as s}
                  <option value={s}>{s}</option>
                {/each}
              </select>
            </td>
            <td class="text-muted">{new Date(p.created_at).toLocaleDateString('en-GB', { day:'2-digit', month:'short' })}</td>
            <td><button class="btn-danger btn-sm" onclick={() => handleDelete(p.id)}>Delete</button></td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .section-label { font-size: 13px; font-weight: 600; color: #334155; margin-bottom: 14px; }
  .amount { font-weight: 600; color: #0f172a; font-variant-numeric: tabular-nums; }
  .id-badge { background: #f1f5f9; color: #64748b; padding: 2px 8px; border-radius: 5px; font-size: 12px; font-family: monospace; }
  .method-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    padding: 3px 10px;
    border-radius: 6px;
    font-size: 12px;
    font-weight: 500;
    color: #475569;
    white-space: nowrap;
  }
  .status-select {
    border: none;
    cursor: pointer;
    font-size: 11px;
    font-weight: 600;
    width: auto;
    padding: 3px 9px;
  }
</style>
