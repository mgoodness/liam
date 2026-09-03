# Agent-behavior eval suite: custom liam-native scenarios first, not SWE-bench

liam gains an agent-behavior eval suite (#106) — hard-assertion scenarios run against liam's existing fake-`Provider` test seam (the same one `agent_test.go` already uses), every PR, zero new infrastructure. This is deliberately **not** SWE-bench or Terminal-Bench, despite both being real, established, adoptable benchmarks with a genuinely clean integration boundary (SWE-bench's harness only consumes a patch-per-task predictions file — agent-agnostic by design). Neither tests what this issue actually names as a goal: safety/instruction adherence — did a `beforeTool` Hook's denial actually stop the model, did `activate_skill` fire correctly. Those are liam-specific integration points no general coding-task benchmark can see. Custom scenarios are required regardless of whether SWE-bench is ever adopted, so they come first.

pi.dev's own `evals` package (the only comparable prior art found across all five surveyed peers — none of pi-go/jcode/kit/crush have anything real) always runs against live provider APIs specifically to catch behavioral drift mocks can't, and supports both hard assertions and judge-model scoring in one framework. liam's v1 takes neither of those yet: fake-`Provider`-only (free, deterministic, catches nothing about real-model drift — an accepted, explicit limitation) and hard-assertions-only (no judge-model API cost/non-determinism to manage yet). Both are real, valuable extensions, deferred to fast-follows rather than bundled into an already low-priority ticket's first cut.

## Considered Options

Adopt SWE-bench/Terminal-Bench as the primary eval mechanism. Rejected for v1: neither can test liam-specific safety/instruction-adherence behavior at all, and both require Python+Docker tooling external to liam's Go single-binary shape — real infrastructure investment better justified once there's actual appetite for it, tracked separately.

Real-provider runs and/or judge-model scoring from the start, matching pi.dev. Rejected for v1: adds cost and non-determinism on top of a suite that doesn't exist yet — ship the free, deterministic tier first.

## Consequences

liam's eval suite initially can't catch real-model behavioral drift or evaluate anything a hard assertion can't check — both real gaps, deliberately accepted for now. Two fast-follows capture what's deferred: extending the custom suite with real-provider + judge-model scoring, and adopting SWE-bench/Terminal-Bench via a thin driver for general coding-task-completion eval.
