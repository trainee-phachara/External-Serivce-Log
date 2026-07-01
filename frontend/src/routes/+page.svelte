<script lang="ts">
  import { onMount } from 'svelte';
  import { getLogs } from '$lib/api';
  import type { LogEntry } from '$lib/types';

  let logs: LogEntry[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let typeFilter = $state('');
  let appFilter = $state('');

  const logTypes = [
    { id: '',         label: 'All' },
    { id: 'request',  label: 'Request' },
    { id: 'response', label: 'Response' },
    { id: 'event',    label: 'Event' },
  ];

  async function load() {
    loading = true; error = '';
    try { logs = await getLogs(typeFilter, appFilter); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed to load logs'; }
    finally { loading = false; }
  }

  function statusClass(code: string) {
    if (code.startsWith('2')) return 'status-2xx';
    if (code.startsWith('4')) return 'status-4xx';
    return 'status-5xx';
  }

  function fmtTime(ts: string) {
    const d = new Date(ts);
    return d.toLocaleTimeString('en-GB') + ' ' + d.toLocaleDateString('en-GB', { day:'2-digit', month:'short' });
  }

  onMount(load);
</script>

<div class="page-header">
  <div>
    <div class="page-title">Logs</div>
    <div class="page-sub">{logs.length} entries{typeFilter ? ` · ${typeFilter}` : ''}</div>
  </div>
  <button class="btn-ghost" onclick={load}>
    ↻ Refresh
  </button>
</div>

{#if error}<div class="alert-error">{error}</div>{/if}

<div class="toolbar card" style="margin-bottom: 16px;">
  <div class="card-body" style="padding: 14px 20px;">
    <div class="toolbar-inner">
      <div class="tabs">
        {#each logTypes as lt}
          <button
            class="tab"
            class:active={typeFilter === lt.id}
            onclick={() => { typeFilter = lt.id; load(); }}
          >{lt.label}</button>
        {/each}
      </div>
      <div class="filter">
        <input
          placeholder="Filter by app..."
          bind:value={appFilter}
          onkeydown={(e) => e.key === 'Enter' && load()}
          style="width: 200px"
        />
        <button class="btn-primary btn-sm" onclick={load}>Search</button>
      </div>
    </div>
  </div>
</div>

<div class="card">
  {#if loading}
    <div class="empty-state"><span class="icon">⏳</span>Loading logs…</div>
  {:else if logs.length === 0}
    <div class="empty-state"><span class="icon">◎</span>No logs found</div>
  {:else}
    <table>
      <thead>
        <tr>
          <th>Time</th>
          <th>App</th>
          <th>Service</th>
          <th>Endpoint</th>
          <th>Status</th>
          <th>Type</th>
          <th>Direction</th>
          <th>Trace ID</th>
        </tr>
      </thead>
      <tbody>
        {#each logs as log}
          <tr>
            <td class="mono text-muted">{fmtTime(log.timestamp)}</td>
            <td><span class="app-chip">{log.source.app_name}</span></td>
            <td class="text-muted">{log.source.service_name}</td>
            <td class="mono">{log.endpoint}</td>
            <td>
              <span class="badge {statusClass(log.http_status)}">{log.http_status}</span>
            </td>
            <td class="text-muted">{log.type}</td>
            <td>
              <span class="dir-badge dir-{log.direction}">{log.direction}</span>
            </td>
            <td class="mono text-muted">{log.trace_id.slice(0, 8)}…</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .toolbar-inner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }
  .tabs { display: flex; gap: 4px; }
  .tab {
    padding: 6px 14px;
    border-radius: 6px;
    border: 1.5px solid #e2e8f0;
    background: transparent;
    color: #64748b;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all .15s;
  }
  .tab:hover { border-color: #cbd5e1; background: #f8fafc; }
  .tab.active { background: #4f46e5; color: white; border-color: #4f46e5; }

  .filter { display: flex; gap: 8px; align-items: center; }

  .app-chip {
    display: inline-block;
    background: #eef2ff;
    color: #4338ca;
    padding: 2px 8px;
    border-radius: 5px;
    font-size: 12px;
    font-weight: 500;
  }

  .dir-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 5px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .04em;
  }
  .dir-inbound  { background: #f0fdf4; color: #166534; }
  .dir-outbound { background: #fff7ed; color: #9a3412; }
</style>
