<script lang="ts">
  import { onMount } from 'svelte';
  import { getUsers, createUser, updateUser, deleteUser } from '$lib/api';
  import type { User } from '$lib/types';

  let users: User[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let name = $state('');
  let email = $state('');
  let creating = $state(false);
  let editId: number | null = $state(null);
  let editName = $state('');
  let editEmail = $state('');

  function avatarColor(id: number) {
    const colors = ['#6366f1','#ec4899','#14b8a6','#f59e0b','#8b5cf6','#06b6d4'];
    return colors[id % colors.length];
  }

  async function load() {
    loading = true; error = '';
    try { users = await getUsers(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed'; }
    finally { loading = false; }
  }

  async function handleCreate() {
    if (!name.trim() || !email.trim()) return;
    creating = true; error = '';
    try { await createUser(name.trim(), email.trim()); name = ''; email = ''; await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed to create user'; }
    finally { creating = false; }
  }

  function startEdit(u: User) { editId = u.id; editName = u.name; editEmail = u.email; }
  function cancelEdit() { editId = null; }

  async function handleUpdate() {
    if (!editId) return; error = '';
    try { await updateUser(editId, editName, editEmail); editId = null; await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed to update'; }
  }

  async function handleDelete(id: number) {
    error = '';
    try { await deleteUser(id); await load(); }
    catch (e: unknown) { error = e instanceof Error ? e.message : 'Failed to delete'; }
  }

  onMount(load);
</script>

<div class="page-header">
  <div>
    <div class="page-title">Users</div>
    <div class="page-sub">{users.length} total</div>
  </div>
  <button class="btn-ghost" onclick={load}>↻ Refresh</button>
</div>

{#if error}<div class="alert-error">{error}</div>{/if}

<div class="card">
  <div class="card-body">
    <div class="section-label">{editId ? 'Edit User' : 'New User'}</div>
    <div class="form-row">
      {#if editId}
        <div class="form-field" style="flex:1; min-width:150px">
          <label class="field-label">Name</label>
          <input bind:value={editName} placeholder="Full name" />
        </div>
        <div class="form-field" style="flex:1; min-width:180px">
          <label class="field-label">Email</label>
          <input bind:value={editEmail} placeholder="email@example.com" type="email" />
        </div>
        <div class="btn-row">
          <button class="btn-primary" onclick={handleUpdate}>Save changes</button>
          <button class="btn-ghost" onclick={cancelEdit}>Cancel</button>
        </div>
      {:else}
        <div class="form-field" style="flex:1; min-width:150px">
          <label class="field-label">Name</label>
          <input bind:value={name} placeholder="Full name" />
        </div>
        <div class="form-field" style="flex:1; min-width:180px">
          <label class="field-label">Email</label>
          <input bind:value={email} placeholder="email@example.com" type="email" />
        </div>
        <div class="btn-row">
          <button class="btn-primary" onclick={handleCreate} disabled={creating}>
            {creating ? 'Creating…' : '+ Create user'}
          </button>
        </div>
      {/if}
    </div>
  </div>
</div>

<div class="card">
  {#if loading}
    <div class="empty-state"><span class="icon">⏳</span>Loading…</div>
  {:else if users.length === 0}
    <div class="empty-state"><span class="icon">◎</span>No users yet — create one above</div>
  {:else}
    <table>
      <thead>
        <tr>
          <th>User</th>
          <th>Email</th>
          <th>ID</th>
          <th>Created</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each users as u}
          <tr class:highlighted={editId === u.id}>
            <td>
              <div class="user-cell">
                <div class="avatar" style="background:{avatarColor(u.id)}">
                  {u.name[0]?.toUpperCase()}
                </div>
                <span class="user-name">{u.name}</span>
              </div>
            </td>
            <td class="text-muted">{u.email}</td>
            <td><span class="id-badge">#{u.id}</span></td>
            <td class="text-muted">{new Date(u.created_at).toLocaleDateString('en-GB', { day:'2-digit', month:'short', year:'numeric' })}</td>
            <td>
              <div class="actions">
                <button class="btn-ghost btn-sm" onclick={() => startEdit(u)}>Edit</button>
                <button class="btn-danger btn-sm" onclick={() => handleDelete(u.id)}>Delete</button>
              </div>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<style>
  .section-label { font-size: 13px; font-weight: 600; color: #334155; margin-bottom: 14px; }
  .btn-row { display: flex; gap: 8px; align-items: flex-end; padding-bottom: 1px; }

  .user-cell { display: flex; align-items: center; gap: 10px; }
  .avatar {
    width: 32px; height: 32px;
    border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    color: white; font-size: 13px; font-weight: 700;
    flex-shrink: 0;
  }
  .user-name { font-weight: 500; color: #0f172a; }
  .id-badge { background: #f1f5f9; color: #64748b; padding: 2px 8px; border-radius: 5px; font-size: 12px; font-family: monospace; }
  tr.highlighted td { background: #fafaff; }
</style>
