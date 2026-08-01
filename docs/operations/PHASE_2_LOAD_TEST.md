# Phase 2 backlog load evidence

**Measured:** 2026-08-01  
**Command:** `npm run load:phase2`  
**Isolation:** disposable PostgreSQL 17 container and per-profile schemas; fake
in-process notification driver; no public network or Mattermost traffic.

The test inserts durable signal and notification backlogs, starts the same incident
evaluator and notification worker implementations used by Core, waits for every
signal to be processed and every notification to reach Delivered, and fails on a
deadlock, worker exit, lost item, or profile budget breach.

| Profile | Signals | Notifications | Observed drain time | Budget |
|---|---:|---:|---:|---:|
| Representative | 1,000 | 250 | 4.52 s | 30 s |
| Explicitly oversized (10×) | 10,000 | 2,500 | 47.41 s | 120 s |

These measurements establish safe single-host starting defaults, not a universal
capacity promise:

- incident workers: concurrency 2, claim batch 50, poll 1 second, lease 30 seconds,
  maximum signal attempts 8;
- notification workers: concurrency 2, poll 1 second, lease 30 seconds, maximum six
  attempts and five-minute retry cap;
- warn when signal or delivery oldest age exceeds 30 seconds; treat two minutes as
  an oversized-profile breach pending deployment-specific objectives; and
- retain incident, timeline, suppression, destination, intent, attempt, certificate,
  and audit correlation evidence indefinitely in Phase 2. No untested automatic
  deletion job is enabled. Capacity planning must preserve at least six months and
  a future retention worker must ship with restore and correlation tests.

Re-run after changing PostgreSQL, worker defaults, indexes, container limits, or
representative site scale. Record runner CPU/memory and the new result rather than
silently widening budgets.
