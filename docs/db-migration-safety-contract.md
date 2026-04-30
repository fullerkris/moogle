# DB Migration Safety Contract

This contract defines safe schema change rules for production releases.

## Principles

- Prefer backward-compatible, multi-step migrations.
- Never require app and DB to switch in one irreversible step.
- Keep rollback paths explicit before deployment.

## Expand/Contract Model

### Phase 1: Expand (safe)

- Add new nullable columns/tables/indexes.
- Dual-write or backfill while old reads still work.
- Do not remove or rename fields in this phase.

### Phase 2: Migrate traffic

- Shift reads to new shape behind feature flag/config.
- Monitor latency/error/data consistency during transition.

### Phase 3: Contract (cleanup)

- Remove deprecated fields only after stable period.
- Contract changes must be isolated from high-risk feature releases.

## Required Pre-Release Checks

- Migration is tagged `expand` or `contract` in release notes.
- Rollback strategy is documented for each migration.
- Data backfill plan exists with stop/resume semantics.
- Compatibility verified in staging with production-like data shape.

## Rollback Rules

- If release fails during expand phase, roll back app first.
- If release includes contract changes, execute DB rollback plan before app rollback when required.
- Never perform destructive data rollback without incident commander approval.

## Prohibited During Release Window

- Dropping populated columns without archived fallback.
- Renaming columns without compatibility layer.
- Large locking migrations without an approved maintenance window.
