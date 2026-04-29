// V5/W6 SETTINGS-V2 — JSON Schema if/then/else evaluator for the plugin
// settings form (Plan C·C2).
//
// The backend (santhosh-tekuri/jsonschema) already validates submitted
// values against the full JSON Schema, including conditional clauses.
// This evaluator is a UX optimization: it lets the admin form HIDE
// fields that are inert under the current values, so the operator does
// not waste time filling in branches that will be ignored.
//
// Supported subset (DESIGN — keep it small on purpose):
//   - top-level   if / then / else        (one layer)
//   - dependentSchemas { K: { properties } } (apply iff values[K] is set)
//   - allOf[*].if/then/else               (in array order, hidden sets union)
//   - leaf condition matchers:
//       properties.<key>.const
//       properties.<key>.enum
//       required: [...]
//       type: '...' (object only — schema-level)
//
// Anything else (oneOf / anyOf / nested-if-inside-if) → ignored by the
// evaluator. Validation still runs server-side, so the worst case is
// "field shown but rejected on save" — never "field hidden but
// silently submitted".
//
// Return contract:
//   - hidden            : keys to skip rendering
//   - requiredOverride  : keys forced required by an apply'd then/else
//                         (currently informational; the form does not
//                         strictly enforce it client-side)
//   - cyclic            : true → caller must show a banner AND treat
//                         hidden as empty (defensive: we already do
//                         that here, but the flag survives the merge)
//
// Hidden semantics (rule 5 in the plan):
//   When an `if` matches, only the keys appearing under the apply'd
//   then/else `properties` participate in the visibility decision for
//   THAT clause. We compute the union of all keys mentioned across all
//   branches of the same if/then/else (the "conditional surface"); a
//   key in the surface is hidden iff it is NOT in the apply'd branch's
//   properties. Keys outside the surface are unaffected.

const MAX_ITERATIONS = 16
const MAX_DEPTH = 2

export interface EvaluatedConditions {
  hidden: Set<string>
  requiredOverride: Set<string>
  cyclic: boolean
}

function emptyResult(cyclic = false): EvaluatedConditions {
  return { hidden: new Set(), requiredOverride: new Set(), cyclic }
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && !Array.isArray(v)
}

function getProperties(node: Record<string, unknown>): Record<string, unknown> | undefined {
  const p = node['properties']
  return isPlainObject(p) ? (p as Record<string, unknown>) : undefined
}

/**
 * Match a single leaf condition object against the current values.
 * Supported clauses:
 *   - type: 'object'       → trivially true (we only handle object schemas)
 *   - required: ['k', ...] → all listed keys must be set in values
 *   - properties.K.const   → values[K] === const
 *   - properties.K.enum    → values[K] ∈ enum
 *
 * Multiple clauses are AND-ed. A condition with no recognised clauses
 * is treated as TRUE (matches the JSON Schema "empty subschema is true"
 * rule).
 */
function matchesCondition(
  cond: Record<string, unknown>,
  values: Record<string, unknown>,
): boolean {
  if (typeof cond['type'] === 'string' && cond['type'] !== 'object') {
    // We only model object schemas; a primitive type clause is
    // unsatisfiable for the values map → does not match.
    return false
  }

  const required = cond['required']
  if (Array.isArray(required)) {
    for (const k of required) {
      if (typeof k !== 'string') return false
      const v = values[k]
      if (v === undefined || v === null || v === '') return false
    }
  }

  const props = getProperties(cond)
  if (props) {
    for (const [key, sub] of Object.entries(props)) {
      if (!isPlainObject(sub)) continue
      const cv = values[key]
      if ('const' in sub) {
        if (cv !== sub['const']) return false
      }
      if ('enum' in sub) {
        const arr = sub['enum']
        if (!Array.isArray(arr) || !arr.some((e) => e === cv)) return false
      }
    }
  }

  return true
}

/**
 * Collect property keys mentioned under a (sub)schema's `properties`.
 * Used both to compute the "conditional surface" and to detect what
 * keys an apply'd branch declares.
 */
function keysOf(node: unknown): string[] {
  if (!isPlainObject(node)) return []
  const p = getProperties(node)
  return p ? Object.keys(p) : []
}

/**
 * Collect the keys an `if` clause READS — used for cycle detection.
 * A cycle exists when the resolution of field A depends on field B
 * AND vice versa.
 */
function dependencyKeys(cond: unknown): string[] {
  if (!isPlainObject(cond)) return []
  const out: string[] = []
  const props = getProperties(cond)
  if (props) out.push(...Object.keys(props))
  const required = cond['required']
  if (Array.isArray(required)) {
    for (const k of required) if (typeof k === 'string') out.push(k)
  }
  return out
}

interface IfClause {
  if: Record<string, unknown>
  then?: Record<string, unknown>
  else?: Record<string, unknown>
}

function readIfClause(node: Record<string, unknown>): IfClause | null {
  const ifNode = node['if']
  if (!isPlainObject(ifNode)) return null
  const thenNode = node['then']
  const elseNode = node['else']
  return {
    if: ifNode as Record<string, unknown>,
    then: isPlainObject(thenNode) ? (thenNode as Record<string, unknown>) : undefined,
    else: isPlainObject(elseNode) ? (elseNode as Record<string, unknown>) : undefined,
  }
}

/**
 * Apply a single if/then/else clause against the current values, given
 * the schema's top-level property keys. Mutates the supplied hidden /
 * requiredOverride sets in place.
 *
 * `depth` guards against `if-inside-then-inside-if` recursion; we cap
 * at MAX_DEPTH (2) — anything deeper is reported as cyclic by the
 * caller (which collapses hidden→empty).
 */
function applyIfClause(
  clause: IfClause,
  values: Record<string, unknown>,
  topKeys: Set<string>,
  hidden: Set<string>,
  requiredOverride: Set<string>,
  depth: number,
): boolean {
  if (depth > MAX_DEPTH) return false

  const matched = matchesCondition(clause.if, values)
  const branch = matched ? clause.then : clause.else
  const otherBranch = matched ? clause.else : clause.then

  // Conditional surface = keys named in EITHER branch's properties.
  // Only these keys are eligible for hide/show by this clause; everything
  // else is left alone.
  const surface = new Set<string>()
  for (const k of keysOf(branch)) surface.add(k)
  for (const k of keysOf(otherBranch)) surface.add(k)

  if (!branch) {
    // No applicable branch (e.g. matched but no `then`): hide every
    // key in the surface that is in the topKeys set, since the schema
    // implicitly says "those fields don't apply".
    for (const k of surface) if (topKeys.has(k)) hidden.add(k)
    return true
  }

  const branchKeys = new Set(keysOf(branch))
  for (const k of surface) {
    if (!topKeys.has(k)) continue
    if (!branchKeys.has(k)) hidden.add(k)
  }

  // Required override: keys explicitly required by the apply'd branch.
  const required = branch['required']
  if (Array.isArray(required)) {
    for (const k of required) {
      if (typeof k === 'string') requiredOverride.add(k)
    }
  }

  return true
}

/**
 * Build a dependency graph: edge B → A means "A's visibility depends
 * on B's value". A cycle in this graph means a field both controls
 * and is controlled by another, which we cannot resolve in a single
 * pass. Caller collapses hidden→empty when this returns true.
 */
function detectCycles(
  schema: Record<string, unknown>,
): boolean {
  const edges = new Map<string, Set<string>>()
  const addEdge = (from: string, to: string) => {
    if (from === to) return
    let s = edges.get(from)
    if (!s) {
      s = new Set()
      edges.set(from, s)
    }
    s.add(to)
  }

  const collectFromClause = (clause: IfClause) => {
    const deps = dependencyKeys(clause.if)
    const targets = [
      ...keysOf(clause.then),
      ...keysOf(clause.else),
    ]
    for (const dep of deps) {
      for (const tgt of targets) addEdge(dep, tgt)
    }
  }

  const top = readIfClause(schema)
  if (top) collectFromClause(top)

  const allOf = schema['allOf']
  if (Array.isArray(allOf)) {
    for (const item of allOf) {
      if (!isPlainObject(item)) continue
      const c = readIfClause(item as Record<string, unknown>)
      if (c) collectFromClause(c)
    }
  }

  const dep = schema['dependentSchemas']
  if (isPlainObject(dep)) {
    for (const [k, sub] of Object.entries(dep)) {
      if (!isPlainObject(sub)) continue
      // dependentSchemas: K controls every key under sub.properties.
      for (const tgt of keysOf(sub)) addEdge(k, tgt)
    }
  }

  // DFS cycle detection.
  const WHITE = 0, GRAY = 1, BLACK = 2
  const color = new Map<string, number>()
  const dfs = (n: string): boolean => {
    color.set(n, GRAY)
    const next = edges.get(n)
    if (next) {
      for (const m of next) {
        const c = color.get(m) ?? WHITE
        if (c === GRAY) return true
        if (c === WHITE && dfs(m)) return true
      }
    }
    color.set(n, BLACK)
    return false
  }
  for (const n of edges.keys()) {
    if ((color.get(n) ?? WHITE) === WHITE && dfs(n)) return true
  }
  return false
}

/**
 * Detect whether any if-clause has a NESTED if inside its then or else
 * beyond MAX_DEPTH. We treat over-deep nesting as a cyclic signal —
 * the form will show all fields and surface the banner so the admin
 * knows to inspect the schema.
 */
function exceedsNesting(schema: Record<string, unknown>): boolean {
  const walk = (node: unknown, depth: number): boolean => {
    if (!isPlainObject(node)) return false
    if (depth > MAX_DEPTH) return true
    const c = readIfClause(node as Record<string, unknown>)
    if (c) {
      if (walk(c.then, depth + 1)) return true
      if (walk(c.else, depth + 1)) return true
    }
    const all = node['allOf']
    if (Array.isArray(all)) {
      for (const item of all) {
        if (walk(item, depth + 1)) return true
      }
    }
    return false
  }
  return walk(schema, 0)
}

export function evaluateConditions(
  schema: Record<string, unknown>,
  values: Record<string, unknown>,
): EvaluatedConditions {
  if (!isPlainObject(schema)) return emptyResult()

  if (detectCycles(schema) || exceedsNesting(schema)) {
    return emptyResult(true)
  }

  const props = getProperties(schema)
  const topKeys = new Set<string>(props ? Object.keys(props) : [])

  const hidden = new Set<string>()
  const requiredOverride = new Set<string>()

  // Hard iteration cap — defensive only; with MAX_DEPTH=2 we converge
  // in a single pass.
  let iter = 0
  while (iter++ < MAX_ITERATIONS) {
    const before = hidden.size + requiredOverride.size

    // 1) Top-level if / then / else
    const top = readIfClause(schema)
    if (top) {
      applyIfClause(top, values, topKeys, hidden, requiredOverride, 1)
    }

    // 2) dependentSchemas
    const dep = schema['dependentSchemas']
    if (isPlainObject(dep)) {
      for (const [k, sub] of Object.entries(dep)) {
        if (!isPlainObject(sub)) continue
        const v = values[k]
        if (v === undefined || v === null || v === '') {
          // Inactive dependency → its keys are hidden iff they appear
          // in the top schema (i.e. they ARE conditional fields).
          for (const tk of keysOf(sub)) {
            if (topKeys.has(tk)) hidden.add(tk)
          }
          continue
        }
        // Active → apply: required goes into requiredOverride.
        const required = (sub as Record<string, unknown>)['required']
        if (Array.isArray(required)) {
          for (const rk of required) {
            if (typeof rk === 'string') requiredOverride.add(rk)
          }
        }
      }
    }

    // 3) allOf[*].if/then/else
    const allOf = schema['allOf']
    if (Array.isArray(allOf)) {
      for (const item of allOf) {
        if (!isPlainObject(item)) continue
        const clause = readIfClause(item as Record<string, unknown>)
        if (clause) {
          applyIfClause(clause, values, topKeys, hidden, requiredOverride, 1)
        }
      }
    }

    if (hidden.size + requiredOverride.size === before) break
  }

  return { hidden, requiredOverride, cyclic: false }
}
