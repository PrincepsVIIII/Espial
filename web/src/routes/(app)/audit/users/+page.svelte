<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { readCookie } from '$lib/auth';
  import {
    problemFrom,
    requestJSONWithMetadata,
    requestVoidWithMetadata,
    type ClientProblem,
  } from '$lib/api/client';
  import type { ManagedUser } from '$lib/api/generated';
  import Timestamp from '$lib/components/Timestamp.svelte';

  let { data } = $props();
  let selected = $state<ManagedUser | null>(null);
  let displayName = $state('');
  let email = $state('');
  let role = $state('');
  let enabled = $state(true);
  let createPassword = $state('');
  let confirmCreatePassword = $state('');
  let resetPassword = $state('');
  let confirmResetPassword = $state('');
  let pending = $state('');
  let problem = $state<ClientProblem | null>(null);
  let receipt = $state<{ message: string; requestID: string } | null>(null);

  function chooseUser(user: ManagedUser) {
    selected = user;
    displayName = user.display_name;
    email = user.email ?? '';
    role = user.roles[0] ?? '';
    enabled = user.enabled;
    resetPassword = '';
    confirmResetPassword = '';
    problem = null;
  }

  function mutationHeaders(extra: Record<string, string> = {}) {
    return {
      'Content-Type': 'application/json',
      'X-CSRF-Token': readCookie('espial_csrf'),
      ...extra,
    };
  }

  function showProblem(error: unknown) {
    problem = problemFrom(error);
    receipt = null;
  }

  async function createUser(event: SubmitEvent) {
    event.preventDefault();
    if (createPassword !== confirmCreatePassword) {
      problem = {
        status: 0,
        code: 'password_mismatch',
        message: 'The new user password confirmation does not match.',
      };
      return;
    }
    const formElement = event.currentTarget as HTMLFormElement;
    const form = new FormData(formElement);
    pending = 'create';
    problem = null;
    try {
      const result = await requestJSONWithMetadata<ManagedUser>(
        fetch,
        '/api/v1/users',
        {
          method: 'POST',
          headers: mutationHeaders(),
          body: JSON.stringify({
            username: form.get('username'),
            display_name: form.get('display_name'),
            email: form.get('email') || undefined,
            role: form.get('role'),
            password: createPassword,
          }),
        },
      );
      receipt = {
        message: `Created ${result.data.username}.`,
        requestID: result.request_id ?? '',
      };
      formElement.reset();
      createPassword = '';
      confirmCreatePassword = '';
      await invalidateAll();
      chooseUser(result.data);
    } catch (error) {
      showProblem(error);
    } finally {
      pending = '';
    }
  }

  async function saveUser(event: SubmitEvent) {
    event.preventDefault();
    if (!selected) return;
    pending = 'save';
    problem = null;
    try {
      const result = await requestJSONWithMetadata<ManagedUser>(
        fetch,
        `/api/v1/users/${selected.id}`,
        {
          method: 'PUT',
          headers: mutationHeaders({
            'If-Match': `"${selected.updated_at}"`,
          }),
          body: JSON.stringify({
            display_name: displayName,
            email,
            role,
            enabled,
          }),
        },
      );
      selected = result.data;
      receipt = {
        message: `Updated ${result.data.username}. Active sessions were revoked if access changed.`,
        requestID: result.request_id ?? '',
      };
      await invalidateAll();
    } catch (error) {
      showProblem(error);
    } finally {
      pending = '';
    }
  }

  async function replacePassword(event: SubmitEvent) {
    event.preventDefault();
    if (!selected) return;
    if (resetPassword !== confirmResetPassword) {
      problem = {
        status: 0,
        code: 'password_mismatch',
        message: 'The replacement password confirmation does not match.',
      };
      return;
    }
    pending = 'password';
    problem = null;
    try {
      const result = await requestVoidWithMetadata(
        fetch,
        `/api/v1/users/${selected.id}/password`,
        {
          method: 'POST',
          headers: mutationHeaders(),
          body: JSON.stringify({ password: resetPassword }),
        },
      );
      resetPassword = '';
      confirmResetPassword = '';
      if (selected.id === data.session?.user.id) {
        await goto(
          `/login?notice=password_changed&request_id=${encodeURIComponent(result.request_id ?? '')}`,
        );
        return;
      }
      receipt = {
        message: `Replaced ${selected.username}'s password and revoked active sessions.`,
        requestID: result.request_id ?? '',
      };
      await invalidateAll();
    } catch (error) {
      showProblem(error);
    } finally {
      pending = '';
    }
  }
</script>

<svelte:head><title>Users · Audit · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <p class="eyebrow">Local access administration</p>
    <h1>Users</h1>
    <p class="page-description">
      Create accounts, assign one role, control access, and replace passwords.
    </p>
  </div>
</header>

<nav class="section-navigation" aria-label="Audit sections">
  <a href="/audit">History</a>
  <a class="active" aria-current="page" href="/audit/users">Users</a>
</nav>

{#if problem ?? data.problem}
  {@const currentProblem = problem ?? data.problem}
  <div class="inline-problem" role="alert">
    <strong>
      {currentProblem?.status === 403
        ? 'User administration denied'
        : 'User administration failed'}
    </strong>
    <span>{currentProblem?.message}</span>
    {#if currentProblem?.request_id}<code>{currentProblem.request_id}</code
      >{/if}
  </div>
{/if}

{#if receipt}
  <div class="mutation-receipt" role="status" aria-live="polite">
    <strong>{receipt.message}</strong>
    {#if receipt.requestID}
      <span>Request ID: <code>{receipt.requestID}</code></span>
      <a
        href={`/audit?correlation_id=${encodeURIComponent(receipt.requestID)}`}
      >
        View matching audit record
      </a>
    {/if}
  </div>
{/if}

{#if !data.problem}
  <div class="user-admin-layout">
    <section class="admin-section" aria-labelledby="user-list-title">
      <div class="operational-section-heading">
        <div>
          <p class="eyebrow">Current access</p>
          <h2 id="user-list-title">Accounts</h2>
        </div>
        <span class="section-count">{data.users.length} shown</span>
      </div>
      {#if data.users.length}
        <div class="table-frame" role="region" aria-label="User accounts table">
          <table class="resource-table user-table">
            <thead>
              <tr>
                <th scope="col">User</th>
                <th scope="col">Role</th>
                <th scope="col">Access</th>
                <th scope="col">Sessions</th>
                <th scope="col">Updated</th>
                <th scope="col">Manage</th>
              </tr>
            </thead>
            <tbody>
              {#each data.users as user}
                <tr>
                  <th scope="row" data-label="User">
                    <span class="resource-name">{user.display_name}</span>
                    <code>{user.username}</code>
                  </th>
                  <td data-label="Role">{user.roles.join(', ')}</td>
                  <td data-label="Access"
                    >{user.enabled ? 'Enabled' : 'Disabled'}</td
                  >
                  <td data-label="Sessions">{user.active_sessions}</td>
                  <td data-label="Updated"
                    ><Timestamp value={user.updated_at} /></td
                  >
                  <td data-label="Manage">
                    <button
                      class="table-action"
                      type="button"
                      aria-pressed={selected?.id === user.id}
                      onclick={() => chooseUser(user)}
                    >
                      Manage
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if data.nextPageURL || data.continuation}
          <nav class="pagination-actions" aria-label="User account pages">
            {#if data.continuation}
              <a class="text-link" href="/audit/users">Return to first page</a>
            {/if}
            {#if data.nextPageURL}
              <a class="next-page-link" href={data.nextPageURL}>Next page</a>
            {/if}
          </nav>
        {/if}
      {:else}
        <div class="empty-state">
          <strong>No local accounts were returned.</strong>
          <span>Create an account with the form on this page.</span>
        </div>
      {/if}
    </section>

    <section
      class="admin-section create-user"
      aria-labelledby="create-user-title"
    >
      <div>
        <p class="eyebrow">New access</p>
        <h2 id="create-user-title">Create local user</h2>
      </div>
      <form class="admin-form" onsubmit={createUser}>
        <label>
          Username
          <input name="username" required maxlength="64" autocomplete="off" />
        </label>
        <label>
          Display name
          <input name="display_name" required maxlength="128" />
        </label>
        <label>
          Email <span class="optional">Optional</span>
          <input name="email" type="email" maxlength="254" />
        </label>
        <label>
          Role
          <select name="role" required>
            <option value="" disabled selected>Select role</option>
            {#each data.roles as availableRole}
              <option value={availableRole.name}>{availableRole.name}</option>
            {/each}
          </select>
        </label>
        <label>
          Password
          <input
            type="password"
            bind:value={createPassword}
            minlength="15"
            maxlength="128"
            autocomplete="new-password"
            required
          />
        </label>
        <label>
          Confirm password
          <input
            type="password"
            bind:value={confirmCreatePassword}
            minlength="15"
            maxlength="128"
            autocomplete="new-password"
            required
          />
        </label>
        <button type="submit" disabled={pending === 'create'}>
          {pending === 'create' ? 'Creating…' : 'Create user'}
        </button>
      </form>
    </section>
  </div>

  {#if selected}
    <section class="user-editor" aria-labelledby="edit-user-title">
      <div class="editor-heading">
        <div>
          <p class="eyebrow">Selected account</p>
          <h2 id="edit-user-title">Manage {selected.username}</h2>
        </div>
        <button
          class="text-button"
          type="button"
          onclick={() => (selected = null)}
        >
          Close
        </button>
      </div>
      <div class="editor-grid">
        <form class="admin-form" onsubmit={saveUser}>
          <h3>Account access</h3>
          <label>
            Display name
            <input bind:value={displayName} required maxlength="128" />
          </label>
          <label>
            Email <span class="optional">Optional</span>
            <input bind:value={email} type="email" maxlength="254" />
          </label>
          <label>
            Role
            <select bind:value={role} required>
              {#each data.roles as availableRole}
                <option value={availableRole.name}>{availableRole.name}</option>
              {/each}
            </select>
          </label>
          <label class="checkbox-field">
            <input type="checkbox" bind:checked={enabled} />
            Account enabled
          </label>
          {#if selected.id === data.session?.user.id}
            <p class="form-note">
              Espial will reject changes that disable your account or remove
              your own administrator access.
            </p>
          {/if}
          <button type="submit" disabled={pending === 'save'}>
            {pending === 'save' ? 'Saving…' : 'Save account'}
          </button>
        </form>
        {#if selected.identity_provider === 'local'}
          <form class="admin-form" onsubmit={replacePassword}>
            <h3>Replace password</h3>
            <p class="form-note">
              This immediately revokes every active session for this account.
            </p>
            <label>
              New password
              <input
                type="password"
                bind:value={resetPassword}
                minlength="15"
                maxlength="128"
                autocomplete="new-password"
                required
              />
            </label>
            <label>
              Confirm new password
              <input
                type="password"
                bind:value={confirmResetPassword}
                minlength="15"
                maxlength="128"
                autocomplete="new-password"
                required
              />
            </label>
            {#if selected.id === data.session?.user.id}
              <p class="form-note">You will be signed out after this change.</p>
            {/if}
            <button type="submit" disabled={pending === 'password'}>
              {pending === 'password' ? 'Replacing…' : 'Replace password'}
            </button>
          </form>
        {:else}
          <div class="admin-form">
            <h3>External identity</h3>
            <p class="form-note">
              Passwords for {selected.identity_provider} accounts are managed by their
              identity provider.
            </p>
          </div>
        {/if}
      </div>
    </section>
  {/if}
{/if}
