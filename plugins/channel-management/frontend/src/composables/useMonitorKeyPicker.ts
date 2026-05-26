import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getSdk } from '../api/sdk'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import type { SdkApiKey } from '@sub2api/plugin-sdk'

/**
 * Channel Monitor "Use my key" picker state machine (plugin version).
 *
 * Mirrors host `useMonitorKeyPicker` but uses SDK `sdk.keys.listActive()` and
 * `sdk.keys.getUserGroupRates()` instead of host-only `keysAPI` / `userGroupsAPI`.
 *
 *   - showKeyPicker / loading / cached active keys / userGroupRates
 *   - openMyKeyPicker: fetches active keys + group rates on first open
 *   - pickMyKey: writes selected key and closes dialog
 *   - closeKeyPicker: closes dialog without selection
 */
export function useMonitorKeyPicker(setApiKey: (key: string) => void) {
  const { t } = useI18n()
  const sdk = getSdk()

  const showKeyPicker = ref(false)
  const myKeysLoading = ref(false)
  const myActiveKeys = ref<SdkApiKey[]>([])
  const userGroupRates = ref<Record<number, number>>({})

  async function openMyKeyPicker() {
    showKeyPicker.value = true
    if (myActiveKeys.value.length > 0) return
    myKeysLoading.value = true
    try {
      const [keys, rates] = await Promise.all([
        sdk.keys.listActive(),
        sdk.keys.getUserGroupRates(),
      ])
      myActiveKeys.value = keys
      userGroupRates.value = rates
    } catch (err: unknown) {
      sdk.notify.error(
        extractApiErrorMessage(err, t('admin.channelMonitor.form.noActiveKey')),
      )
    } finally {
      myKeysLoading.value = false
    }
  }

  function pickMyKey(k: SdkApiKey) {
    setApiKey(k.key)
    showKeyPicker.value = false
  }

  function closeKeyPicker() {
    showKeyPicker.value = false
  }

  return {
    showKeyPicker,
    myKeysLoading,
    myActiveKeys,
    userGroupRates,
    openMyKeyPicker,
    pickMyKey,
    closeKeyPicker,
  }
}
