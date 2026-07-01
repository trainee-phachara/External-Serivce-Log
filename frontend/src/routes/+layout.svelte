<script lang="ts">
  import { page } from '$app/state';
  let { children } = $props();

  const nav = [
    { href: '/',         label: 'Logs',     icon: '▦' },
    { href: '/users',    label: 'Users',    icon: '◎' },
    { href: '/orders',   label: 'Orders',   icon: '◈' },
    { href: '/payments', label: 'Payments', icon: '◆' },
  ];
</script>

<div class="app">
  <aside class="sidebar">
    <div class="brand">
      <span class="brand-icon">◉</span>
      <span class="brand-text">ServiceLog</span>
    </div>

    <nav>
      {#each nav as item}
        <a href={item.href} class:active={page.url.pathname === item.href}>
          <span class="nav-icon">{item.icon}</span>
          {item.label}
        </a>
      {/each}
    </nav>

    <div class="sidebar-footer">
      <div class="status-dot"></div>
      <span>Demo mode</span>
    </div>
  </aside>

  <main>
    {@render children()}
  </main>
</div>

<style>
  /* ── Reset & Tokens ─────────────────────────── */
  :global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', sans-serif;
    background: #f1f5f9;
    color: #0f172a;
    font-size: 14px;
    line-height: 1.5;
  }

  /* ── Table ──────────────────────────────────── */
  :global(table) { width: 100%; border-collapse: collapse; }
  :global(thead th) {
    background: #f8fafc;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .06em;
    color: #94a3b8;
    padding: 11px 16px;
    border-bottom: 1px solid #e2e8f0;
    text-align: left;
    white-space: nowrap;
  }
  :global(tbody td) {
    padding: 13px 16px;
    border-bottom: 1px solid #f1f5f9;
    color: #334155;
    vertical-align: middle;
  }
  :global(tbody tr:last-child td) { border-bottom: none; }
  :global(tbody tr:hover td) { background: #fafbff; }

  /* ── Inputs ─────────────────────────────────── */
  :global(input), :global(select), :global(textarea) {
    padding: 8px 12px;
    border: 1.5px solid #e2e8f0;
    border-radius: 8px;
    font-size: 14px;
    width: 100%;
    background: white;
    outline: none;
    color: #0f172a;
    transition: border-color .15s, box-shadow .15s;
  }
  :global(input:focus), :global(select:focus) {
    border-color: #6366f1;
    box-shadow: 0 0 0 3px rgba(99,102,241,.12);
  }
  :global(input::placeholder) { color: #94a3b8; }

  /* ── Buttons ────────────────────────────────── */
  :global(button) {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 16px;
    border: none;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all .15s;
    white-space: nowrap;
  }
  :global(.btn-primary) { background: #4f46e5; color: white; }
  :global(.btn-primary:hover) { background: #4338ca; transform: translateY(-1px); box-shadow: 0 4px 12px rgba(79,70,229,.3); }
  :global(.btn-danger)  { background: #ef4444; color: white; }
  :global(.btn-danger:hover) { background: #dc2626; }
  :global(.btn-ghost)   { background: white; color: #334155; border: 1.5px solid #e2e8f0; }
  :global(.btn-ghost:hover) { background: #f8fafc; border-color: #cbd5e1; }
  :global(.btn-sm) { padding: 5px 12px; font-size: 12px; border-radius: 6px; }
  :global(button:disabled) { opacity: .5; cursor: not-allowed; transform: none !important; box-shadow: none !important; }

  /* ── Badges ─────────────────────────────────── */
  :global(.badge) {
    display: inline-flex;
    align-items: center;
    padding: 3px 9px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: .02em;
  }
  :global(.badge-pending)    { background: #fef3c7; color: #92400e; }
  :global(.badge-processing) { background: #dbeafe; color: #1e40af; }
  :global(.badge-shipped)    { background: #ede9fe; color: #5b21b6; }
  :global(.badge-delivered)  { background: #d1fae5; color: #065f46; }
  :global(.badge-completed)  { background: #d1fae5; color: #065f46; }
  :global(.badge-cancelled)  { background: #fee2e2; color: #991b1b; }
  :global(.badge-failed)     { background: #fee2e2; color: #991b1b; }
  :global(.badge-refunded)   { background: #f3e8ff; color: #6b21a8; }
  :global(.status-2xx) { background: #d1fae5; color: #065f46; }
  :global(.status-4xx) { background: #fef3c7; color: #92400e; }
  :global(.status-5xx) { background: #fee2e2; color: #991b1b; }

  /* ── Cards ───────────────────────────────────── */
  :global(.card) {
    background: white;
    border-radius: 12px;
    border: 1px solid #e2e8f0;
    box-shadow: 0 1px 3px rgba(0,0,0,.05), 0 1px 2px rgba(0,0,0,.03);
    margin-bottom: 20px;
    overflow: hidden;
  }
  :global(.card-body) { padding: 20px 24px; }

  /* ── Misc ────────────────────────────────────── */
  :global(.empty-state) {
    padding: 64px 24px;
    text-align: center;
    color: #94a3b8;
    font-size: 14px;
  }
  :global(.empty-state .icon) { font-size: 32px; display: block; margin-bottom: 12px; opacity: .4; }
  :global(.field-label) { display: block; font-size: 12px; font-weight: 600; color: #64748b; margin-bottom: 5px; text-transform: uppercase; letter-spacing: .04em; }
  :global(.form-grid) { display: grid; gap: 14px; }
  :global(.form-row) { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
  :global(.form-field) { display: flex; flex-direction: column; }
  :global(.page-header) { display: flex; align-items: center; justify-content: space-between; margin-bottom: 24px; }
  :global(.page-title) { font-size: 22px; font-weight: 700; color: #0f172a; }
  :global(.page-sub)   { font-size: 13px; color: #94a3b8; margin-top: 2px; }
  :global(.mono) { font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 12px; }
  :global(.text-muted) { color: #94a3b8; }
  :global(.actions) { display: flex; gap: 6px; }
  :global(.alert-error) {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 10px 14px;
    border-radius: 8px;
    font-size: 13px;
    margin-bottom: 16px;
  }
  :global(select) { appearance: auto; }

  /* ── Layout ──────────────────────────────────── */
  .app {
    display: flex;
    min-height: 100vh;
  }

  .sidebar {
    width: 220px;
    flex-shrink: 0;
    background: #0f172a;
    display: flex;
    flex-direction: column;
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    z-index: 20;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 20px 18px 18px;
    border-bottom: 1px solid rgba(255,255,255,.07);
  }
  .brand-icon { font-size: 20px; color: #818cf8; }
  .brand-text { font-size: 16px; font-weight: 700; color: white; letter-spacing: -.02em; }

  nav {
    flex: 1;
    padding: 12px 10px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  nav a {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    border-radius: 8px;
    color: #94a3b8;
    text-decoration: none;
    font-size: 14px;
    font-weight: 500;
    transition: all .15s;
  }
  nav a:hover { background: rgba(255,255,255,.06); color: #e2e8f0; }
  nav a.active { background: rgba(99,102,241,.15); color: #a5b4fc; }
  .nav-icon { font-size: 14px; width: 18px; text-align: center; }

  .sidebar-footer {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 14px 18px;
    border-top: 1px solid rgba(255,255,255,.07);
    font-size: 12px;
    color: #475569;
  }
  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #22c55e;
    box-shadow: 0 0 6px #22c55e;
  }

  main {
    margin-left: 220px;
    flex: 1;
    padding: 32px;
    max-width: calc(1200px + 220px);
  }
</style>
