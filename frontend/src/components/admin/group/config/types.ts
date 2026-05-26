import type { AdminGroup } from '@/types'

export interface GroupConfigProps {
  mode: 'create' | 'edit'
  platform: string
  formData: Record<string, any>
  groups: AdminGroup[]
  editingGroupId?: number | null
}

export interface GroupConfigExposed {
  getRoutingRulesApiFormat?(): Record<string, number[]> | null
  loadRoutingRules?(data: Record<string, number[]> | null): Promise<void>
  resetRoutingRules?(): void
}
