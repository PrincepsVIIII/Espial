<script lang="ts">
  import { invalidate } from '$app/navigation';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import { administrativeMutation } from '$lib/administration';
  import { problemFrom } from '$lib/api/client';
  import type {
    AdministrativeMutationReceipt,
    MaintenanceWindow,
    MaintenanceWindowWrite,
    NotificationSilence,
    NotificationSilenceWrite,
  } from '$lib/api/generated';

  let { data } = $props();
  let selectedWindow = $state<MaintenanceWindow | null>(null);
  let selectedSilence = $state<NotificationSilence | null>(null);
  let problem = $state<ReturnType<typeof problemFrom> | null>(null);
  let receipt = $state<AdministrativeMutationReceipt | null>(null);
  let pending = $state('');

  const localInputTime = (value: Date) =>
    new Date(value.getTime() - value.getTimezoneOffset() * 60_000)
      .toISOString()
      .slice(0, 16);
  const initialStart = () => localInputTime(new Date(Date.now() + 5 * 60_000));
  const initialEnd = () => localInputTime(new Date(Date.now() + 65 * 60_000));
  const inputTime = (value?: string) =>
    value ? localInputTime(new Date(value)) : '';
  const utc = (value: FormDataEntryValue | null) =>
    new Date(String(value ?? '')).toISOString();

  function targetLabel(silence: NotificationSilence): string {
    if (silence.incident_id) return `Incident ${silence.incident_id}`;
    if (silence.rule_id) return `Rule ${silence.rule_id}`;
    return `Resource ${silence.resource_id}`;
  }

  async function saveWindow(event: SubmitEvent) {
    event.preventDefault();
    pending = 'window';
    problem = null;
    const values = new FormData(event.currentTarget as HTMLFormElement);
    const text = (name: string) => String(values.get(name) ?? '').trim();
    const body: MaintenanceWindowWrite = {
      reason: text('reason'),
      starts_at: utc(values.get('starts_at')),
      ends_at: utc(values.get('ends_at')),
      enabled: values.get('enabled') === 'on',
      ...(text('integration_id')
        ? { integration_id: text('integration_id') }
        : {}),
      ...(text('resource_id') ? { resource_id: text('resource_id') } : {}),
      ...(text('check_type') ? { check_type: text('check_type') } : {}),
    };
    try {
      receipt = await administrativeMutation(
        selectedWindow
          ? `/api/v1/maintenance-windows/${selectedWindow.id}`
          : '/api/v1/maintenance-windows',
        selectedWindow ? 'PUT' : 'POST',
        body,
        selectedWindow?.version,
      );
      selectedWindow = null;
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }

  async function saveSilence(event: SubmitEvent) {
    event.preventDefault();
    pending = 'silence';
    problem = null;
    const values = new FormData(event.currentTarget as HTMLFormElement);
    const target = String(values.get('target') ?? '').split(':');
    const body: NotificationSilenceWrite = {
      reason: String(values.get('reason') ?? '').trim(),
      starts_at: utc(values.get('starts_at')),
      expires_at: utc(values.get('expires_at')),
      enabled: values.get('enabled') === 'on',
      ...(target[0] === 'incident' ? { incident_id: target[1] } : {}),
      ...(target[0] === 'rule' ? { rule_id: target[1] } : {}),
      ...(target[0] === 'resource' ? { resource_id: target[1] } : {}),
    };
    try {
      receipt = await administrativeMutation(
        selectedSilence
          ? `/api/v1/silences/${selectedSilence.id}`
          : '/api/v1/silences',
        selectedSilence ? 'PUT' : 'POST',
        body,
        selectedSilence?.version,
      );
      selectedSilence = null;
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }

  async function revoke(
    kind: 'maintenance-windows' | 'silences',
    item: MaintenanceWindow | NotificationSilence,
  ) {
    pending = item.id;
    problem = null;
    try {
      receipt = await administrativeMutation(
        `/api/v1/${kind}/${item.id}/revoke`,
        'POST',
        undefined,
        item.version,
      );
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }
</script>

<svelte:head><title>Alert suppressions · Espial</title></svelte:head>
<header class="page-header">
  <div>
    <p class="eyebrow">Time-bounded controls</p>
    <h1>Suppressions</h1>
    <p class="page-description">
      Maintenance changes effective health while preserving raw failures.
      Silences suppress later notification intents only; incidents remain
      visible and unchanged.
    </p>
  </div>
</header>
<nav class="section-navigation" aria-label="Alert views">
  <a href="/alerts">Active</a><a href="/alerts/history">History</a><a
    href="/alerts/rules">Rules</a
  ><a class="active" aria-current="page" href="/alerts/suppressions"
    >Suppressions</a
  >
</nav>

{#if problem ?? data.problem}{@const currentProblem = problem ?? data.problem}
  <div class="inline-problem" role="alert">
    <strong>Suppression operation failed</strong><span
      >{currentProblem?.message}</span
    >{#if currentProblem?.request_id}<code>{currentProblem.request_id}</code
      >{/if}
  </div>{/if}
{#if receipt}<div class="mutation-receipt" role="status">
    <strong>Control change committed.</strong><span
      >Request ID: <code>{receipt.request_id}</code></span
    >{#if receipt.audit_url}<a href={receipt.audit_url}
        >View matching audit record</a
      >{/if}
  </div>{/if}

{#if !(problem ?? data.problem)}
  <section class="admin-section" aria-labelledby="maintenance-title">
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Effective health control</p>
        <h2 id="maintenance-title">Maintenance windows</h2>
      </div>
      <span class="section-count">{data.windows.length} shown</span>
    </div>
    {#if data.windows.length}<div class="table-frame">
        <table class="resource-table">
          <thead
            ><tr
              ><th>Reason</th><th>Scope</th><th>Range (UTC)</th><th>State</th
              ><th>Manage</th></tr
            ></thead
          ><tbody
            >{#each data.windows as window}<tr
                ><th
                  ><span class="resource-name">{window.reason}</span><small
                    >Created by {window.created_by_name}</small
                  ></th
                ><td
                  ><code
                    >{window.resource_id ??
                      window.integration_id ??
                      window.check_type}</code
                  ></td
                ><td
                  ><Timestamp value={window.starts_at} /> → <Timestamp
                    value={window.ends_at}
                  /></td
                ><td
                  >{window.revoked_at
                    ? 'Revoked'
                    : window.expired_at
                      ? 'Expired'
                      : window.enabled
                        ? 'Enabled'
                        : 'Disabled'}</td
                ><td class="table-actions"
                  ><button
                    class="table-action"
                    type="button"
                    disabled={!!window.revoked_at}
                    onclick={() => (selectedWindow = window)}>Edit</button
                  ><button
                    class="table-action"
                    type="button"
                    disabled={!!window.revoked_at || pending === window.id}
                    onclick={() => revoke('maintenance-windows', window)}
                    >Revoke</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{:else}<div class="empty-state">
        <strong>No maintenance windows.</strong><span
          >Planned work can be scoped below.</span
        >
      </div>{/if}
  </section>
  <section class="admin-section" aria-labelledby="window-editor-title">
    <div class="editor-heading">
      <div>
        <p class="eyebrow">
          {selectedWindow ? 'Replace control' : 'New control'}
        </p>
        <h2 id="window-editor-title">
          {selectedWindow ? 'Edit maintenance window' : 'Schedule maintenance'}
        </h2>
      </div>
      {#if selectedWindow}<button
          class="text-button"
          type="button"
          onclick={() => (selectedWindow = null)}>Create instead</button
        >{/if}
    </div>
    {#key selectedWindow?.id ?? 'new-window'}<form
        class="admin-form suppression-form"
        onsubmit={saveWindow}
      >
        <label
          >Reason<textarea name="reason" required maxlength="512"
            >{selectedWindow?.reason ?? ''}</textarea
          ></label
        ><label
          >Integration <span class="optional">Optional</span><select
            name="integration_id"
            ><option value="">Any integration</option
            >{#each data.integrations as integration}<option
                value={integration.id}
                selected={selectedWindow?.integration_id === integration.id}
                >{integration.display_name}</option
              >{/each}</select
          ></label
        ><label
          >Resource <span class="optional">Optional</span><select
            name="resource_id"
            ><option value="">Any resource</option
            >{#each data.resources as resource}<option
                value={resource.id}
                selected={selectedWindow?.resource_id === resource.id}
                >{resource.display_name}</option
              >{/each}</select
          ></label
        ><label
          >Check type <span class="optional">Optional</span><input
            name="check_type"
            pattern="[a-z][a-z0-9_.-]*"
            value={selectedWindow?.check_type ?? ''}
          /></label
        ><label
          >Starts (local time; stored as UTC)<input
            name="starts_at"
            type="datetime-local"
            required
            value={inputTime(selectedWindow?.starts_at) || initialStart()}
          /></label
        ><label
          >Ends (local time; stored as UTC)<input
            name="ends_at"
            type="datetime-local"
            required
            value={inputTime(selectedWindow?.ends_at) || initialEnd()}
          /></label
        ><label class="checkbox-field"
          ><input
            name="enabled"
            type="checkbox"
            checked={selectedWindow?.enabled ?? true}
          /> Enabled</label
        >
        <p class="form-note">
          Provide at least one integration, resource, or check-type scope.
        </p>
        <button type="submit" disabled={pending === 'window'}
          >{pending === 'window'
            ? 'Saving…'
            : selectedWindow
              ? 'Replace window'
              : 'Schedule window'}</button
        >
      </form>{/key}
  </section>
  <section class="admin-section" aria-labelledby="silence-title">
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Notification decision control</p>
        <h2 id="silence-title">Silences</h2>
      </div>
      <span class="section-count">{data.silences.length} shown</span>
    </div>
    {#if data.silences.length}<div class="table-frame">
        <table class="resource-table">
          <thead
            ><tr
              ><th>Reason</th><th>Target</th><th>Expires (UTC)</th><th>State</th
              ><th>Manage</th></tr
            ></thead
          ><tbody
            >{#each data.silences as silence}<tr
                ><th
                  ><span class="resource-name">{silence.reason}</span><small
                    >Created by {silence.created_by_name}</small
                  ></th
                ><td><code>{targetLabel(silence)}</code></td><td
                  ><Timestamp value={silence.expires_at} /></td
                ><td
                  >{silence.revoked_at
                    ? 'Revoked'
                    : silence.expired_at
                      ? 'Expired'
                      : silence.enabled
                        ? 'Enabled'
                        : 'Disabled'}</td
                ><td class="table-actions"
                  ><button
                    class="table-action"
                    type="button"
                    disabled={!!silence.revoked_at}
                    onclick={() => (selectedSilence = silence)}>Edit</button
                  ><button
                    class="table-action"
                    type="button"
                    disabled={!!silence.revoked_at || pending === silence.id}
                    onclick={() => revoke('silences', silence)}>Revoke</button
                  ></td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{:else}<div class="empty-state">
        <strong>No notification silences.</strong><span
          >Incidents remain visible whether or not a silence matches.</span
        >
      </div>{/if}
  </section>

  <section class="admin-section" aria-labelledby="silence-editor-title">
    <div class="editor-heading">
      <div>
        <p class="eyebrow">
          {selectedSilence ? 'Replace control' : 'New control'}
        </p>
        <h2 id="silence-editor-title">
          {selectedSilence ? 'Edit silence' : 'Create silence'}
        </h2>
      </div>
      {#if selectedSilence}<button
          class="text-button"
          type="button"
          onclick={() => (selectedSilence = null)}>Create instead</button
        >{/if}
    </div>
    {#key selectedSilence?.id ?? 'new-silence'}<form
        class="admin-form suppression-form"
        onsubmit={saveSilence}
      >
        <label
          >Reason<textarea name="reason" required maxlength="512"
            >{selectedSilence?.reason ?? ''}</textarea
          ></label
        ><label
          >Target<select name="target" required
            ><option value="" disabled selected={!selectedSilence}
              >Select exact target</option
            ><optgroup label="Active incidents"
              >{#each data.incidents as incident}<option
                  value={`incident:${incident.id}`}
                  selected={selectedSilence?.incident_id === incident.id}
                  >{incident.title}</option
                >{/each}</optgroup
            ><optgroup label="Rules"
              >{#each data.rules as rule}<option
                  value={`rule:${rule.id}`}
                  selected={selectedSilence?.rule_id === rule.id}
                  >{rule.name}</option
                >{/each}</optgroup
            ><optgroup label="Resources"
              >{#each data.resources as resource}<option
                  value={`resource:${resource.id}`}
                  selected={selectedSilence?.resource_id === resource.id}
                  >{resource.display_name}</option
                >{/each}</optgroup
            ></select
          ></label
        ><label
          >Starts (local time; stored as UTC)<input
            name="starts_at"
            type="datetime-local"
            required
            value={inputTime(selectedSilence?.starts_at) || initialStart()}
          /></label
        ><label
          >Expires (local time; stored as UTC)<input
            name="expires_at"
            type="datetime-local"
            required
            value={inputTime(selectedSilence?.expires_at) || initialEnd()}
          /></label
        ><label class="checkbox-field"
          ><input
            name="enabled"
            type="checkbox"
            checked={selectedSilence?.enabled ?? true}
          /> Enabled</label
        >
        <p class="form-note">
          Silencing never changes health, incident status, or incident
          visibility.
        </p>
        <button type="submit" disabled={pending === 'silence'}
          >{pending === 'silence'
            ? 'Saving…'
            : selectedSilence
              ? 'Replace silence'
              : 'Create silence'}</button
        >
      </form>{/key}
  </section>
{/if}
