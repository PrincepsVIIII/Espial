<script lang="ts">
  let {
    eyebrow,
    title,
    description,
    phase,
    planned,
  }: {
    eyebrow: string;
    title: string;
    description: string;
    phase: string;
    planned: string[];
  } = $props();
</script>

<header class="page-header">
  <div>
    <p class="eyebrow">{eyebrow}</p>
    <h1>{title}</h1>
  </div>
  <span class="state-label state-unknown">Not available</span>
</header>

<section class="state-panel" aria-labelledby="availability-title">
  <div class="state-panel-summary">
    <p class="eyebrow">{phase}</p>
    <h2 id="availability-title">
      This section is not connected to domain data yet.
    </h2>
    <p>{description}</p>
  </div>
  <div class="planned-scope" id="planned-scope">
    <h3>Planned scope</h3>
    <ul>
      {#each planned as item}
        <li>{item}</li>
      {/each}
    </ul>
  </div>
</section>
