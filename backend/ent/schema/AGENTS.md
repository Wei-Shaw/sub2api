# ENT SCHEMA KNOWLEDGE BASE

## OVERVIEW
`backend/ent/schema/` is the handwritten Ent enclave: entity definitions, mixins, edges, indexes, and annotations.
Edit schema here; generated files elsewhere under `backend/ent/` are outputs, not authoring surfaces.

## STRUCTURE
```text
backend/ent/schema/
├── *.go              # Entity schemas
└── mixins/
    ├── soft_delete.go
    └── time.go
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Add/change entity fields | `*.go` in this directory | Entity-specific fields, edges, indexes, annotations |
| Shared timestamps | `mixins/time.go` | Common created/updated fields |
| Soft delete behavior | `mixins/soft_delete.go` | Custom interceptor/hook-based implementation |
| Code generation entry | `../generate.go` | `go:generate` feature list |
| Matching SQL migration | `../../migrations/` | Schema change should usually ship with migration |

## CONVENTIONS
- Author schema changes here, not in generated Ent output.
- Reuse the local mixins where appropriate; this repo has a custom `SoftDeleteMixin` and a shared `TimeMixin`.
- Use annotations/indexes deliberately; table naming and SQL behavior are part of the handwritten schema contract.
- Treat schema edits and migration edits as one workflow, not separate chores.

## ANTI-PATTERNS
- Editing `backend/ent/*.go` generated files instead of changing schema and regenerating.
- Introducing a schema change without the corresponding SQL migration when runtime data shape changes.
- Replacing the repo’s custom soft-delete pattern with Ent’s default soft-delete approach.
- Forgetting regeneration after schema edits.

## COMMANDS
```bash
cd backend && go generate ./ent
cd backend && go generate ./cmd/server
```

## NOTES
- The parent `backend/` file owns the broad “generated Ent code is off-limits” rule; this child focuses on the handwritten authoring surface.
- `SoftDeleteMixin` is a repo-specific behavior boundary; read it before changing delete semantics.
- If a schema edit changes query semantics or constraints, check `internal/repository/` callers and add/update a migration.
