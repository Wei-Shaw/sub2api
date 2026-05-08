import type { AdminGroup } from '@/types'

export interface GroupConfigProps {
  mode: 'create' | 'edit'
  platform: string
  formData: Record<string, any>
  groups: AdminGroup[]
  editingGroupId?: number | null
}

export interface GroupConfigExposed {
  validate?(): { valid: boolean; error?: string }
}
