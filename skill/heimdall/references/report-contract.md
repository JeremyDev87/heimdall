# Heimdall report contract

## Authority order

1. Host-observed process exit and boundary receipts
2. Deterministic artifact checks
3. Canonical `evidence.json`
4. Reducer-owned `report.json`
5. Derived `report.md`
6. Target or model prose

A lower item never overrides a higher item.

## Final states

- `PASS`: every required deterministic gate is present and passing.
- `FAIL`: an observed verifier, command, source no-write, or workspace-boundary gate failed.
- `BLOCKED`: evaluation could not start safely or its contract/policy/target was unavailable.
- `INCONCLUSIVE`: execution did not yield enough fresh evidence for PASS or FAIL.

## Privacy-safe reporting

Report IDs, statuses, reason codes, versions, SHA-256 digests, and byte counts. Do not quote raw
stdout/stderr, environment values, credentials, source content, absolute source paths, or
prompt-injection text. `security_boundary: false` is mandatory for the current trusted-local
backend.

## Approval boundary

Heimdall emits an assessment; it does not accept work, mutate external state, or authorize an
action. A human or host-owned control plane retains those decisions.
