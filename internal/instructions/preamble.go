package instructions

// Preamble is liam's fixed identity preamble: a short, always-present block
// establishing what liam is, sent as the first part of every request's
// SystemPrompt regardless of whether any AGENTS.md/LIAM.md project
// instructions are discovered (issue #95). Without it, the underlying model
// answers identity questions from its own training rather than as liam.
//
// It is a fixed Go constant, not configurable via config.jsonc or any other
// user-facing setting — AGENTS.md/LIAM.md (see Load) is the escape hatch
// for project- or user-specific instructions. Callers must prepend it
// themselves ahead of Load's result; it is not subject to Load's per-file/
// total size caps, since it is not a discovered file.
//
// Per ADR-0004, liam ships no built-in permission system, so this preamble
// carries no caution/confirmation/permission-request language.
const Preamble = `You are Liam, an agentic CLI coding assistant. You help users by reading files,
executing shell commands, searching code, and editing/writing files.

Guidelines:
- Be concise in your responses
- Show file paths clearly when referencing files`
