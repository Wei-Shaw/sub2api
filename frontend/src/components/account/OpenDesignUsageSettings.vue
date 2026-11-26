<template>
  <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600">
    <h3 class="text-sm font-semibold">{{ t('admin.accounts.opendesignUsage.title') }}</h3>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.opendesignUsage.deviceHint') }}</p>
    <OpenDesignUsageCell :account="account" @updated="emit('updated', $event)" />
    <button v-if="!device" type="button" class="btn btn-primary btn-sm"
      :disabled="busy || startingDevice" data-test="opendesign-start-device" @click="startDevice">
      {{ t('admin.accounts.opendesignUsage.authorize') }}
    </button>
    <div v-else class="space-y-1 text-sm" aria-live="polite">
      <a :href="device.activate_url" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:underline">
        {{ t('admin.accounts.opendesignUsage.openAuthorization') }}
      </a>
      <p>{{ t('admin.accounts.opendesignUsage.deviceCode', { code: device.user_code }) }}</p>
    </div>
    <label class="block">
      <span class="input-label">{{ t('admin.accounts.opendesignUsage.token') }}</span>
      <input v-model.trim="token" type="password" class="input" maxlength="522" autocomplete="off" placeholder="vela_ctrl_…" />
    </label>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.opendesignUsage.hint') }}</p>
    <div class="flex flex-wrap gap-2">
      <button type="button" class="btn btn-secondary btn-sm" data-test="opendesign-bind-token"
        :disabled="busy || startingDevice || !!device || !token" @click="bind">
        {{ t('admin.accounts.opendesignUsage.bind') }}
      </button>
      <button v-if="account.opendesign_usage" type="button" class="btn btn-secondary btn-sm"
        :disabled="busy || startingDevice || !!device" @click="unbind">
        {{ t('admin.accounts.opendesignUsage.unbind') }}
      </button>
    </div>
    <p v-if="errorKey" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ t(errorKey) }}</p>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, OpenDesignDevicePrompt, OpenDesignUsageState } from '@/types'
import OpenDesignUsageCell from './OpenDesignUsageCell.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OpenDesignUsageState | undefined] }>()
const { t } = useI18n()
const token = ref('')
const busy = ref(false)
const errorKey = ref('')
const device = ref<OpenDesignDevicePrompt>()
const startingDevice = ref(false)
let deviceController: AbortController | undefined
let pollTimer: ReturnType<typeof setTimeout> | undefined

function stopDevice() {
  deviceController?.abort()
  deviceController = undefined
  clearTimeout(pollTimer)
  device.value = undefined
  startingDevice.value = false
}

onBeforeUnmount(stopDevice)

watch(() => props.account.id, () => {
  stopDevice()
  token.value = ''
  errorKey.value = ''
})

async function startDevice() {
  if (startingDevice.value || device.value || busy.value) return
  const id = props.account.id
  const controller = new AbortController()
  deviceController = controller
  startingDevice.value = true
  errorKey.value = ''
  try {
    const prompt = await adminAPI.accounts.startOpenDesignDeviceAuthorization(id, controller.signal)
    if (controller.signal.aborted) return
    device.value = prompt
    scheduleDevicePoll(id, prompt, controller)
  } catch {
    if (!controller.signal.aborted) {
      stopDevice()
      errorKey.value = 'admin.accounts.opendesignUsage.deviceFailed'
    }
  } finally {
    if (deviceController === controller) startingDevice.value = false
  }
}

function scheduleDevicePoll(id: number, prompt: OpenDesignDevicePrompt, controller: AbortController, failures = 0) {
  const interval = Math.max(2, prompt.poll_interval) * 1000
  pollTimer = setTimeout(async () => {
    if (controller.signal.aborted) return
    if (Date.now() >= Date.parse(prompt.expires_at)) {
      stopDevice()
      errorKey.value = 'admin.accounts.opendesignUsage.deviceExpired'
      return
    }
    try {
      const result = await adminAPI.accounts.pollOpenDesignDeviceAuthorization(id, prompt.attempt_id, controller.signal)
      if (controller.signal.aborted) return
      if (result.status === 'pending') {
        scheduleDevicePoll(id, prompt, controller)
        return
      }
      stopDevice()
      if (result.status === 'bound' && result.usage) {
        emit('updated', result.usage)
      } else {
        errorKey.value = result.status === 'expired'
          ? 'admin.accounts.opendesignUsage.deviceExpired'
          : 'admin.accounts.opendesignUsage.deviceFailed'
      }
    } catch (error: unknown) {
      if (controller.signal.aborted) return
      const failure = error as { reason?: string; status?: number } | null
      if (failures < 2 && (failure?.status === 0 || failure?.status === 429 || failure?.status === 503)) {
        scheduleDevicePoll(id, prompt, controller, failures + 1)
        return
      }
      stopDevice()
      errorKey.value = failure?.reason === 'OPENDESIGN_DEVICE_EXPIRED'
        ? 'admin.accounts.opendesignUsage.deviceExpired'
        : 'admin.accounts.opendesignUsage.deviceFailed'
    }
  }, interval * (2 ** failures))
}

async function bind() {
  if (busy.value || !token.value) return
  const id = props.account.id
  busy.value = true
  errorKey.value = ''
  try {
    const state = await adminAPI.accounts.bindOpenDesignUsage(id, token.value)
    if (props.account.id === id) emit('updated', state)
  } catch (error: unknown) {
    if (props.account.id !== id) return
    const reason = (error as { reason?: string } | null)?.reason
    errorKey.value = reason === 'OPENDESIGN_USAGE_UNAUTHORIZED'
      ? 'admin.accounts.opendesignUsage.unauthorized'
      : 'admin.accounts.opendesignUsage.bindFailed'
  } finally {
    token.value = ''
    busy.value = false
  }
}

async function unbind() {
  if (busy.value) return
  const id = props.account.id
  busy.value = true
  errorKey.value = ''
  try {
    await adminAPI.accounts.unbindOpenDesignUsage(id)
    if (props.account.id === id) emit('updated', undefined)
  } catch {
    if (props.account.id === id) errorKey.value = 'admin.accounts.opendesignUsage.unbindFailed'
  } finally {
    busy.value = false
  }
}
</script>
