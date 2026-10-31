import { useI18n } from 'vue-i18n'
import type { Permission } from '@/api/admin/adminAccessKeys'

// 权限 ID 是稳定协议；后端标签和分类可能始终是中文，不作为本地化回退值。
export function useAdminAccessKeyPermissions() {
  const { t, te } = useI18n()
  function permissionLabel(permission: Permission): string {
    const key = `adminAccessKeys.permissions.${permission.id.replace(/_/g, '_u').replace(/\./g, '_d')}.label`
    return te(key) ? t(key) : t('adminAccessKeys.unknownPermission', { id: permission.id })
  }
  function permissionCategory(permission: Permission): string {
    const key = `adminAccessKeys.permissions.${permission.id.replace(/_/g, '_u').replace(/\./g, '_d')}.category`
    return te(key) ? t(key) : t('adminAccessKeys.other')
  }
  return { permissionLabel, permissionCategory }
}
