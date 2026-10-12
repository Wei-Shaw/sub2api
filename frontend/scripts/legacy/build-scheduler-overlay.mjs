import fs from 'node:fs'
import path from 'node:path'
import crypto from 'node:crypto'
import ts from 'typescript'
import { JSDOM } from 'jsdom'

if (process.env.BUAGA_ALLOW_LEGACY_OVERLAY !== '1') {
  throw new Error('Archived version-specific overlay. Use pnpm verify:release for current releases.')
}

const dist = path.resolve('../backend/internal/web/dist')
const assets = path.join(dist, 'assets')
const namespace = 'scheduler-refinement-r1'
const target = path.join(assets, namespace)
const schedulerName = 'SchedulerMonitorView-DJKX4gd_.js'
if (!fs.existsSync(path.join(assets, schedulerName))) {
  throw new Error('The historical scheduler bundle is absent; no assets were changed.')
}
const helperSource = fs.readFileSync('src/views/admin/scheduler/schedulerRecovery.ts', 'utf8')
const helper = ts.transpileModule(helperSource, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const locale = '(r("admin.ops.scheduler.cooling")==="临时冷却"?"zh":"en")'
const replacements = new Map([
  ['V((e=C.value)==null?void 0:e.model_cooldowns.length)', 'V(__brCount(C.value?.model_cooldowns??[]))'],
  ['r("admin.ops.scheduler.activeCooldowns")', `(${locale}==="zh"?"受影响账号":"Affected accounts")`],
  ['r("admin.ops.scheduler.cooldownHint")', `(${locale}==="zh"?"冷却或恢复验证中的账号":"Accounts cooling down or verifying recovery")`],
  ['o(r)("admin.ops.scheduler.cooldownHint")', `(${locale}==="zh"?"冷却或恢复验证中的账号":"Accounts cooling down or verifying recovery")`],
  ['o(r)("admin.ops.scheduler.cooldownListHint")', `(${locale}==="zh"?"故障范围与恢复进度":"Failure scope and recovery progress")`],
  ['o(r)("admin.ops.scheduler.remaining")', `(e.state&&e.state!=="cooldown"?(${locale}==="zh"?"验证进度":"Recovery progress"):r("admin.ops.scheduler.remaining"))`],
  ['a(e.model)', `a((e.model==="*"?(${locale}==="zh"?"整个账号":"Entire account"):e.model)+" · "+__brReason(e,${locale}))`],
  ['a(o(r)("admin.ops.scheduler.cooling"))', `a(__brState(e,${locale}))`],
  ['a(Q(e.remaining_seconds))', `a(e.state&&e.state!=="cooldown"?(e.recovery_successes||0)+"/2 · "+(${locale}==="zh"?"进行中 ":"In flight ")+(e.probes_in_flight||0):Q(e.remaining_seconds))`]
])
const matched = new Map([...replacements.keys()].map(key => [key, 0]))

function rewrite(code, file, scheduler = false) {
  const source = ts.createSourceFile(file, code, ts.ScriptTarget.Latest, true, ts.ScriptKind.JS)
  if (source.parseDiagnostics.length) throw new Error(`Invalid input ${file}`)
  const edits = []
  const visit = node => {
    const text = node.getText(source)
    if (scheduler && replacements.has(text)) {
      matched.set(text, matched.get(text) + 1)
      edits.push([node.getStart(source), node.end, replacements.get(text)])
      return
    }
    if (ts.isStringLiteral(node) && node.text.startsWith('assets/')) {
      edits.push([node.getStart(source), node.end, JSON.stringify(`assets/${namespace}/${node.text.slice(7)}`)])
    } else ts.forEachChild(node, visit)
  }
  visit(source)
  for (const [start, end, value] of edits.sort((a, b) => b[0] - a[0])) code = code.slice(0, start) + value + code.slice(end)
  if (scheduler) code = 'import {affectedAccountCount as __brCount,recoveryStateLabel as __brState,recoveryReasonLabel as __brReason} from "./schedulerRecovery.js";\n' + code
  if (ts.createSourceFile(file, code, ts.ScriptTarget.Latest, true, ts.ScriptKind.JS).parseDiagnostics.length) throw new Error(`Invalid output ${file}`)
  return code
}

// Keep the approved bundles intact. A versioned asset directory ensures all
// relative/cyclic imports share one module identity without stale immutable caches.
const originals = fs.readdirSync(assets).filter(name => fs.statSync(path.join(assets, name)).isFile())
fs.mkdirSync(target, { recursive: true })
for (const name of originals) {
  const from = path.join(assets, name)
  const to = path.join(target, name)
  if (name.endsWith('.js')) fs.writeFileSync(to, rewrite(fs.readFileSync(from, 'utf8'), name, name === schedulerName))
  else fs.copyFileSync(from, to)
}
for (const [expression, count] of matched) if (count !== 1) throw new Error(`Expected one AST match, got ${count}: ${expression}`)
fs.writeFileSync(path.join(target, 'schedulerRecovery.js'), helper)
const document = new JSDOM(fs.readFileSync(path.join(dist, 'index.html'), 'utf8'))
for (const node of document.window.document.querySelectorAll('[src], [href]')) {
  for (const name of ['src', 'href']) {
    const value = node.getAttribute(name)
    if (value?.startsWith('/assets/') && !value.startsWith(`/assets/${namespace}/`)) node.setAttribute(name, `/assets/${namespace}/${value.slice(8)}`)
  }
}
fs.writeFileSync(path.join(dist, 'index.html'), document.serialize())
const manifest = originals.map(name => ({name, sha256: crypto.createHash('sha256').update(fs.readFileSync(path.join(assets, name))).digest('hex')}))
fs.writeFileSync(path.join(dist, 'scheduler-overlay-manifest.json'), JSON.stringify({namespace, originals: manifest, matched: Object.fromEntries(matched)}, null, 2))
console.log(JSON.stringify({namespace, preservedAssets: originals.length, schedulerExpressions: matched.size}))
