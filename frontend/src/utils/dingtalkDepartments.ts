import type { DingTalkDepartment } from '@/api/dingtalk'

export function buildDingTalkDepartmentRows(ds: DingTalkDepartment[]) {
  const byID = new Map(ds.map(d => [d.id, d]))
  const children = new Map<number, typeof ds>()
  for (const d of ds) { const list = children.get(d.parent_id) || []; list.push(d); children.set(d.parent_id, list) }
  const result: { id: number; name: string; depth: number; hasChildren: boolean }[] = []
  const seen = new Set<number>()
  const stack = ds.filter(d => !byID.has(d.parent_id) || d.id === d.parent_id).reverse().map(d => ({ d, depth: 0 }))
  while (stack.length) {
    const { d, depth } = stack.pop()!
    if (seen.has(d.id)) continue
    seen.add(d.id)
    const nested = (children.get(d.id) || []).filter(c => c.id !== d.id)
    result.push({ id: d.id, name: d.name, depth, hasChildren: nested.length > 0 })
    for (let i = nested.length - 1; i >= 0; i--) stack.push({ d: nested[i]!, depth: depth + 1 })
  }
  return result
}

// Stop at the visible scope boundary; never invent inaccessible ancestors.
export function dingTalkDepartmentPath(departments: DingTalkDepartment[], id: number): DingTalkDepartment[] {
  const byID = new Map(departments.map(d => [d.id, d]))
  const path: DingTalkDepartment[] = []
  const seen = new Set<number>()
  let department = byID.get(id)
  while (department && !seen.has(department.id)) {
    seen.add(department.id)
    path.push(department)
    department = byID.get(department.parent_id)
  }
  return path.reverse()
}
