import { defineStore } from 'pinia'
import { ref } from 'vue'
import settingsAPI from '@/api/admin/settings'

export const useLdapSettingsStore = defineStore('ldapSettings', () => {
  const isTesting = ref(false)
  const isSyncing = ref(false)

  const testConnection = async () => {
    isTesting.value = true
    try {
      const data = await settingsAPI.testLDAPConnection()
      return data
    } finally {
      isTesting.value = false
    }
  }

  const syncNow = async () => {
    isSyncing.value = true
    try {
      const data = await settingsAPI.syncLDAPUsersNow()
      return data
    } finally {
      isSyncing.value = false
    }
  }

  return {
    isTesting,
    isSyncing,
    testConnection,
    syncNow
  }
})
