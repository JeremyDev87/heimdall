# Heimdall

Heimdall is an evidence-first evaluator for agent harnesses.

It inspects the paths, boundaries, tools, state transitions, and verification evidence that shape an agent's behavior. The project is intentionally minimal: deterministic checks come before scores, and observed evidence comes before self-reported success.

## Status

Early development. The repository currently contains only the project brief; implementation scope will be added incrementally.

## Initial direction

- evaluate the harness around an agent, not only the underlying model;
- treat authorization, scope, and side effects as first-class checks;
- preserve reproducible evidence for every finding;
- fail closed when required evidence is missing or contradictory;
- keep the evaluator smaller than the harness it evaluates.
