<script lang="ts">
  import { invalidate } from '$app/navigation';
  import AlertNavigation from '$lib/components/AlertNavigation.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import { administrativeMutation } from '$lib/administration';
  import { problemFrom, requestJSON } from '$lib/api/client';
  import { readCookie } from '$lib/auth';
  import type {
    AdministrativeMutationReceipt,
    IncidentRuleAPIView,
    IncidentRulePrecedencePreview,
    IncidentRuleWrite,
  } from '$lib/api/generated';

  let { data } = $props();
  let selected = $state<IncidentRuleAPIView | null>(null);
  let pending = $state(false);
  let problem = $state<ReturnType<typeof problemFrom> | null>(null);
  let receipt = $state<AdministrativeMutationReceipt | null>(null);
  let preview = $state<IncidentRulePrecedencePreview | null>(null);
  let conditions = $state<IncidentRuleWrite['conditions']>([
    {
      state: 'critical',
      severity: 'critical',
      min_occurrences: 1,
      for_seconds: 0,
    },
  ]);

  function selectRule(rule: IncidentRuleAPIView) {
    selected = rule;
    conditions = rule.conditions.map((condition) => ({ ...condition }));
    problem = null;
    receipt = null;
  }

  function clearEditor() {
    selected = null;
    conditions = [
      {
        state: 'critical',
        severity: 'critical',
        min_occurrences: 1,
        for_seconds: 0,
      },
    ];
  }

  function addCondition() {
    if (conditions.length < 4)
      conditions = [
        ...conditions,
        {
          state: 'warning',
          severity: 'warning',
          min_occurrences: 1,
          for_seconds: 0,
        },
      ];
  }

  async function saveRule(event: SubmitEvent) {
    event.preventDefault();
    pending = true;
    problem = null;
    const values = new FormData(event.currentTarget as HTMLFormElement);
    const text = (name: string) => String(values.get(name) ?? '').trim();
    const body: IncidentRuleWrite = {
      name: text('name'),
      enabled: values.get('enabled') === 'on',
      priority: Number(values.get('priority')),
      conditions: conditions.map((condition) => ({ ...condition })),
      recovery_state: text(
        'recovery_state',
      ) as IncidentRuleWrite['recovery_state'],
      recovery_min_occurrences: Number(values.get('recovery_min_occurrences')),
      recovery_for_seconds: Number(values.get('recovery_for_seconds')),
      ...(text('integration_id')
        ? { integration_id: text('integration_id') }
        : {}),
      ...(text('resource_id') ? { resource_id: text('resource_id') } : {}),
      ...(text('resource_kind')
        ? { resource_kind: text('resource_kind') }
        : {}),
      ...(text('check_type') ? { check_type: text('check_type') } : {}),
      ...(text('reason_code') ? { reason_code: text('reason_code') } : {}),
    };
    try {
      receipt = await administrativeMutation(
        selected
          ? `/api/v1/incident-rules/${selected.id}`
          : '/api/v1/incident-rules',
        selected ? 'PUT' : 'POST',
        body,
        selected?.version,
      );
      await invalidate('espial:monitoring');
      clearEditor();
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = false;
    }
  }

  async function previewRules(event: SubmitEvent) {
    event.preventDefault();
    pending = true;
    problem = null;
    const values = new FormData(event.currentTarget as HTMLFormElement);
    const resourceID = String(values.get('resource_id') ?? '');
    const resource = data.resources.find((item) => item.id === resourceID);
    try {
      preview = await requestJSON<IncidentRulePrecedencePreview>(
        fetch,
        '/api/v1/incident-rules/preview',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': readCookie('espial_csrf'),
          },
          body: JSON.stringify({
            integration_id: resource?.integration_id ?? '',
            resource_id: resourceID,
            check_type: String(values.get('check_type') ?? ''),
            state: String(values.get('state') ?? ''),
            reason_code: String(values.get('reason_code') ?? ''),
          }),
        },
      );
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = false;
    }
  }
</script>

<svelte:head><title>Alert rules · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <h1>Alert rules</h1>
    <p class="page-description">
      Create, replace, enable, and explain the exact rule applied to a
      normalized health signal.
    </p>
  </div>
</header>
<AlertNavigation permissions={data.session?.user.permissions ?? []} />

{#if problem ?? data.problem}{@const currentProblem = problem ?? data.problem}
  <div class="inline-problem" role="alert">
    <strong>Rule operation failed</strong><span>{currentProblem?.message}</span
    >{#if currentProblem?.request_id}<code>{currentProblem.request_id}</code
      >{/if}
  </div>{/if}
{#if receipt}<div class="mutation-receipt" role="status">
    <strong>Rule change committed.</strong><span
      >Request ID: <code>{receipt.request_id}</code></span
    >{#if receipt.audit_url}<a href={receipt.audit_url}
        >View matching audit record</a
      >{/if}
  </div>{/if}

{#if !(problem ?? data.problem)}
  <div class="rule-admin-layout">
    <section class="admin-section" aria-labelledby="rule-list-title">
      <div class="operational-section-heading">
        <div>
          <h2 id="rule-list-title">Current rules</h2>
        </div>
        <span class="section-count">{data.rules.length} shown</span>
      </div>
      {#if data.rules.length}<div class="table-frame">
          <table class="resource-table">
            <thead
              ><tr
                ><th>Rule</th><th>Priority</th><th>Scope</th><th>Conditions</th
                ><th>Updated</th><th>Manage</th></tr
              ></thead
            ><tbody
              >{#each data.rules as rule}<tr
                  ><th
                    ><span class="resource-name">{rule.name}</span><small
                      >{rule.enabled ? 'Enabled' : 'Disabled'}</small
                    ></th
                  ><td>{rule.priority}</td><td
                    ><code
                      >{rule.resource_id ??
                        rule.integration_id ??
                        rule.resource_kind ??
                        rule.check_type ??
                        'global'}</code
                    ></td
                  ><td>{rule.conditions.length}</td><td
                    ><Timestamp value={rule.updated_at} /></td
                  ><td
                    ><button
                      class="table-action"
                      type="button"
                      onclick={() => selectRule(rule)}>Manage</button
                    ></td
                  ></tr
                >{/each}</tbody
            >
          </table>
        </div>{:else}<div class="empty-state">
          <strong>No rules returned.</strong><span
            >Create a rule to begin incident evaluation.</span
          >
        </div>{/if}
    </section>

    <section class="admin-section" aria-labelledby="rule-editor-title">
      <div class="editor-heading">
        <div>
          <h2 id="rule-editor-title">{selected?.name ?? 'Create rule'}</h2>
        </div>
        {#if selected}<button
            type="button"
            class="text-button"
            onclick={clearEditor}>Create instead</button
          >{/if}
      </div>
      {#key selected?.id ?? 'new'}
        <form class="admin-form rule-form" onsubmit={saveRule}>
          <label
            >Name<input
              name="name"
              required
              maxlength="128"
              value={selected?.name ?? ''}
            /></label
          >
          <label
            >Priority<input
              name="priority"
              type="number"
              min="0"
              max="10000"
              required
              value={selected?.priority ?? 100}
            /></label
          >
          <label class="checkbox-field"
            ><input
              name="enabled"
              type="checkbox"
              checked={selected?.enabled ?? true}
            /> Enabled</label
          >
          <label
            >Integration <span class="optional">Optional</span><select
              name="integration_id"
              ><option value="">Any integration</option
              >{#each data.integrations as integration}<option
                  value={integration.id}
                  selected={selected?.integration_id === integration.id}
                  >{integration.display_name}</option
                >{/each}</select
            ></label
          >
          <label
            >Resource <span class="optional">Optional</span><select
              name="resource_id"
              ><option value="">Any resource</option
              >{#each data.resources as resource}<option
                  value={resource.id}
                  selected={selected?.resource_id === resource.id}
                  >{resource.display_name}</option
                >{/each}</select
            ></label
          >
          <label
            >Resource kind <span class="optional">Optional</span><input
              name="resource_kind"
              pattern="[a-z][a-z0-9_.-]*"
              value={selected?.resource_kind ?? ''}
            /></label
          >
          <label
            >Check type <span class="optional">Optional</span><input
              name="check_type"
              pattern="[a-z][a-z0-9_.-]*"
              value={selected?.check_type ?? ''}
            /></label
          >
          <label
            >Reason code <span class="optional">Optional</span><input
              name="reason_code"
              pattern="[a-z][a-z0-9_.-]*"
              value={selected?.reason_code ?? ''}
            /></label
          >
          <fieldset>
            <legend>Failure conditions</legend
            >{#each conditions as condition, index}<div class="condition-row">
                <label
                  >State<select bind:value={condition.state}
                    ><option value="warning">Warning</option><option
                      value="critical">Critical</option
                    ><option value="unknown">Unknown</option><option
                      value="stale">Stale</option
                    ></select
                  ></label
                ><label
                  >Severity<select bind:value={condition.severity}
                    ><option value="warning">Warning</option><option
                      value="critical">Critical</option
                    ></select
                  ></label
                ><label
                  >Occurrences<input
                    type="number"
                    min="1"
                    max="1000"
                    bind:value={condition.min_occurrences}
                  /></label
                ><label
                  >For seconds<input
                    type="number"
                    min="0"
                    max="2592000"
                    bind:value={condition.for_seconds}
                  /></label
                ><button
                  type="button"
                  class="text-button"
                  onclick={() =>
                    (conditions = conditions.filter(
                      (_, itemIndex) => itemIndex !== index,
                    ))}>Remove</button
                >
              </div>{/each}<button
              type="button"
              class="text-button"
              disabled={conditions.length >= 4}
              onclick={addCondition}>Add condition</button
            >
          </fieldset>
          <label
            >Recovery state<select name="recovery_state"
              ><option
                value="healthy"
                selected={(selected?.recovery_state ?? 'healthy') === 'healthy'}
                >Healthy</option
              ><option
                value="warning"
                selected={selected?.recovery_state === 'warning'}
                >Warning</option
              ><option
                value="unknown"
                selected={selected?.recovery_state === 'unknown'}
                >Unknown</option
              ></select
            ></label
          >
          <label
            >Recovery occurrences<input
              name="recovery_min_occurrences"
              type="number"
              min="1"
              max="1000"
              required
              value={selected?.recovery_min_occurrences ?? 2}
            /></label
          >
          <label
            >Recovery for seconds<input
              name="recovery_for_seconds"
              type="number"
              min="0"
              max="2592000"
              required
              value={selected?.recovery_for_seconds ?? 0}
            /></label
          >
          <button type="submit" disabled={pending || conditions.length === 0}
            >{pending
              ? 'Saving…'
              : selected
                ? 'Replace rule'
                : 'Create rule'}</button
          >
        </form>
      {/key}
    </section>
  </div>

  <section
    class="admin-section preview-section"
    aria-labelledby="preview-title"
  >
    <h2 id="preview-title">Explain rule precedence</h2>
    <form class="admin-form compact-admin-form" onsubmit={previewRules}>
      <label
        >Resource<select name="resource_id" required
          ><option value="" disabled selected>Select resource</option
          >{#each data.resources as resource}<option value={resource.id}
              >{resource.display_name}</option
            >{/each}</select
        ></label
      ><label
        >Check type<input
          name="check_type"
          required
          pattern="[a-z][a-z0-9_.-]*"
        /></label
      ><label
        >State<select name="state"
          ><option value="critical">Critical</option><option value="warning"
            >Warning</option
          ><option value="unknown">Unknown</option><option value="stale"
            >Stale</option
          ></select
        ></label
      ><label
        >Reason code <span class="optional">Optional</span><input
          name="reason_code"
          pattern="[a-z][a-z0-9_.-]*"
        /></label
      ><button type="submit" disabled={pending}>Preview</button>
    </form>
    {#if preview}<div class="decision-explanation" role="status">
        <strong>{preview.explanation}</strong
        >{#each preview.candidates as candidate}<p>
            <code>{candidate.name}</code> — {candidate.explanation}
          </p>{/each}
      </div>{/if}
  </section>
{/if}
