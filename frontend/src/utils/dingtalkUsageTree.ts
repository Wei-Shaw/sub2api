import type { DingTalkStatistics, DingTalkUsageRow } from '@/api/dingtalk'

export interface DingTalkUsageNode {
  key: string
  value: DingTalkUsageRow
  children: DingTalkUsageNode[]
}
export interface DingTalkUsageTreeRow extends DingTalkUsageRow {
  key: string
  rootKey: string
  depth: number
  rank: string
  hasChildren: boolean
}

export function buildDingTalkUsageTree(data: DingTalkStatistics, mode: 'companies' | 'departments' | 'users'): DingTalkUsageNode[] {
  const node = (key: string, value: DingTalkUsageRow): DingTalkUsageNode => ({ key, value, children: [] })
  if (mode === 'users') return data.users.map(u => node(`user:${u.id}`, u))
  const companies = new Map(data.companies.map(c => [c.id, node(`company:${c.id}`, c)]))
  const departments = new Map(data.departments.map(d => [`${d.app_id}:${d.id}`, node(`department:${d.app_id}:${d.id}`, d)]))
  const parents = new Map<string, string>(data.organizations.flatMap(a => a.departments.map(d => [`${a.id}:${d.id}`, `${a.id}:${d.parent_id}`] as const)))
  const companyIDs = new Map(data.organizations.map(a => [a.id, a.company_id]))
  const roots: DingTalkUsageNode[] = []
  for (const [key, entry] of departments) {
    const parent = departments.get(parents.get(key) || '')
    if (parent && parent !== entry) parent.children.push(entry)
    else if (mode === 'companies') companies.get(companyIDs.get(entry.value.app_id || '') || '')?.children.push(entry)
    else roots.push(entry)
  }
  const result = mode === 'companies' ? [...companies.values()] : roots
  const pending = [result]
  while (pending.length) {
    const siblings = pending.pop()!
    siblings.sort((a, b) => b.value.cost - a.value.cost || a.key.localeCompare(b.key))
    for (const entry of siblings) if (entry.children.length) pending.push(entry.children)
  }
  return result
}

// Searching keeps ancestors, so a matching department is never detached from its company.
export function flattenDingTalkUsageTree(roots: DingTalkUsageNode[], expanded: Set<string>, query: string): DingTalkUsageTreeRow[] {
  const all: DingTalkUsageTreeRow[] = []
  const stack = roots.map((node, i) => ({ node, depth: 0, rank: `${i + 1}`, rootKey: node.key })).reverse()
  const seen = new Set<string>()
  while (stack.length) {
    const { node, depth, rank, rootKey } = stack.pop()!
    if (seen.has(node.key)) continue
    seen.add(node.key)
    all.push({ ...node.value, key: node.key, rootKey, depth, rank, hasChildren: node.children.length > 0 })
    for (let i = node.children.length - 1; i >= 0; i--) stack.push({ node: node.children[i]!, depth: depth + 1, rank: `${rank}.${i + 1}`, rootKey })
  }
  const search = query.trim().toLocaleLowerCase()
  if (search) {
    const keep = new Set<string>()
    const ancestors: string[] = []
    for (const row of all) {
      ancestors.length = row.depth
      if (`${row.name} ${row.id}`.toLocaleLowerCase().includes(search)) { keep.add(row.key); ancestors.forEach(key => keep.add(key)) }
      ancestors.push(row.key)
    }
    return all.filter(row => keep.has(row.key))
  }
  let hiddenBelow = Infinity
  return all.filter(row => {
    if (row.depth > hiddenBelow) return false
    hiddenBelow = expanded.has(row.key) ? Infinity : row.depth
    return true
  })
}
