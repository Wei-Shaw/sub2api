/**
 * Minimal group shape for group config shared components.
 *
 * This is a subset of host's AdminGroup type -- plugins don't have access
 * to the full type definition. The host passes the full AdminGroup objects
 * which are structurally compatible with this minimal interface.
 */
export interface GroupConfigGroup {
  id: number
  name: string
  platform: string
  status: string
  subscription_type?: string
  claude_code_only?: boolean
  fallback_group_id_on_invalid_request?: number | null
  [key: string]: unknown
}

export interface GroupConfigProps {
  mode: 'create' | 'edit'
  platform: string
  formData: Record<string, unknown>
  groups: GroupConfigGroup[]
  editingGroupId?: number | null
}

export interface GroupConfigExposed {
  getRoutingRulesApiFormat?(): Record<string, number[]> | null
  loadRoutingRules?(data: Record<string, number[]> | null): Promise<void>
  resetRoutingRules?(): void
}
