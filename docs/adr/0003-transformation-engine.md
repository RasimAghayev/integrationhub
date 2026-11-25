# ADR-003: Transformation Engine

## Status
Proposed

## Context
Field mappings must transform API response keys into internal model fields. Three transform modes are required: visual mapping (drag-and-drop), template-based, and code scripts.

## Decision
Support three transform types:
- Visual: FieldMapping list with JSONPath source paths and transform rules, applied programmatically
- Template: text/template Go template string rendered against the parsed response
- Code: gopher-lua script executed in a sandboxed Lua environment against the response

Transform rules (visual mode):
- none: pass through
- uppercase/lowercase: string case conversion
- format_date: parse and reformat timestamps
- compute: arithmetic on numeric fields
- lookup: map value via lookup table
- regex: extract/replace via regex

JSONPath extraction uses gval supporting $, ., [], *, .., filters; extracts single values or arrays.

## Consequences
- Visual mappings cover most cases without code
- Templates cover flexible formatting without a scripting language
- Lua covers complex logic but requires sandboxing (CPU/memory limits, no file/network access)
- All transform types must be idempotent (same input → same output)

## Alternatives Considered
- JavaScript (QuickJS/otto): familiar but heavier; Lua chosen for lightweight sandboxing
- JMESPath vs gval: gval chosen for Go-native JSONPath with filter support
- Compiled Go plugins: rejected due to security and distribution complexity
