<template>
  <BaseDialog
    :show="show"
    :title="t('keys.bulkEdit.title')"
    width="normal"
    :close-on-escape="!submitting"
    :show-close-button="!submitting"
    @close="close"
  >
    <form id="bulk-edit-keys-form" class="space-y-5" @submit.prevent="submit">
      <div class="space-y-1 text-sm">
        <p class="font-medium text-gray-900 dark:text-white">
          {{ t('keys.bulkEdit.selectedCount', { count: pendingKeys.length }) }}
        </p>
        <p class="text-gray-500 dark:text-gray-400">{{ t('keys.bulkEdit.hint') }}</p>
      </div>

      <fieldset :disabled="submitting" class="space-y-5">
        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.group_id" type="checkbox" class="checkbox" data-test="enable-group" />
            {{ t('keys.groupLabel') }}
          </label>
          <Select
            v-if="enabled.group_id"
            v-model="groupId"
            :options="groupOptions"
            :placeholder="t('keys.selectGroup')"
            :disabled="submitting"
            :aria-label="t('keys.groupLabel')"
            searchable
            data-test="group-input"
          />
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.status" type="checkbox" class="checkbox" data-test="enable-status" />
            {{ t('keys.statusLabel') }}
          </label>
          <Select
            v-if="enabled.status"
            v-model="status"
            :options="statusOptions"
            :disabled="submitting"
            :aria-label="t('keys.statusLabel')"
            data-test="status-input"
          />
        </div>

        <div v-for="field in limitFields" :key="field.key" class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input
              v-model="enabled[field.key]"
              type="checkbox"
              class="checkbox"
              :data-test="`enable-${field.key}`"
            />
            {{ t(field.label) }}
          </label>
          <div v-if="enabled[field.key]">
            <input
              v-model="limits[field.key]"
              type="number"
              min="0"
              step="any"
              required
              class="input"
              :aria-label="t(field.label)"
              :data-test="`${field.key}-input`"
            />
            <p class="input-hint">{{ t('keys.bulkEdit.limitHint') }}</p>
          </div>
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.expires_at" type="checkbox" class="checkbox" data-test="enable-expiration" />
            {{ t('keys.expiration') }}
          </label>
          <div v-if="enabled.expires_at" class="space-y-2">
            <label class="flex items-center gap-2 text-sm">
              <input v-model="neverExpires" type="checkbox" class="checkbox" data-test="never-expires" />
              {{ t('keys.noExpiration') }}
            </label>
            <input
              v-if="!neverExpires"
              v-model="expirationDate"
              type="datetime-local"
              required
              class="input"
              :aria-label="t('keys.expirationDate')"
              data-test="expiration-input"
            />
          </div>
        </div>

        <div v-for="field in ipFields" :key="field.key" class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input
              v-model="enabled[field.key]"
              type="checkbox"
              class="checkbox"
              :data-test="`enable-${field.key}`"
            />
            {{ t(field.label) }}
          </label>
          <div v-if="enabled[field.key]">
            <textarea
              v-model="ipLists[field.key]"
              rows="3"
              class="input font-mono text-sm"
              :aria-label="t(field.label)"
              :data-test="`${field.key}-input`"
            />
            <p class="input-hint">{{ t('keys.bulkEdit.ipHint') }}</p>
          </div>
        </div>
      </fieldset>

      <p v-if="validationError" role="alert" class="text-sm text-red-600 dark:text-red-400">
        {{ validationError }}
      </p>
      <div v-if="failures.length" role="alert" class="space-y-2 text-sm text-red-600 dark:text-red-400">
        <p>{{ t('keys.bulkEdit.failureHint') }}</p>
        <ul class="max-h-40 space-y-1 overflow-y-auto">
          <li v-for="failure in failures" :key="failure.id" class="break-words">
            #{{ failure.id }} {{ failure.name }}: {{ failure.message }}
          </li>
        </ul>
      </div>
    </form>

    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="submitting" @click="close">
        {{ t('common.cancel') }}
      </button>
      <button
        type="submit"
        form="bulk-edit-keys-form"
        class="btn btn-primary"
        :disabled="!canSubmit"
        data-test="submit"
      >
        {{ submitting ? t('keys.saving') : t('keys.bulkEdit.apply', { count: pendingKeys.length }) }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { ApiKey, Group, UpdateApiKeyRequest } from '@/types'

type SelectedKey = Pick<ApiKey, 'id' | 'name'>
type LimitField = 'quota' | 'rate_limit_5h' | 'rate_limit_1d' | 'rate_limit_7d'
type IPField = 'ip_whitelist' | 'ip_blacklist'
type EditableField = LimitField | IPField | 'group_id' | 'status' | 'expires_at'

const props = defineProps<{
  show: boolean
  selectedKeys: SelectedKey[]
  groups: Group[]
}>()
const emit = defineEmits<{
  close: []
  updated: [succeededIds: number[]]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const submitting = ref(false)
const pendingKeys = ref<SelectedKey[]>([])
const failures = ref<Array<{ id: number; name: string; message: string }>>([])
const enabled = reactive<Record<EditableField, boolean>>({
  group_id: false,
  status: false,
  quota: false,
  rate_limit_5h: false,
  rate_limit_1d: false,
  rate_limit_7d: false,
  expires_at: false,
  ip_whitelist: false,
  ip_blacklist: false
})
const groupId = ref<number | null>(null)
const status = ref<'active' | 'inactive'>('active')
const limits = reactive<Record<LimitField, string | number>>({
  quota: '', rate_limit_5h: '', rate_limit_1d: '', rate_limit_7d: ''
})
const ipLists = reactive<Record<IPField, string>>({ ip_whitelist: '', ip_blacklist: '' })
const neverExpires = ref(false)
const expirationDate = ref('')
const limitFields: Array<{ key: LimitField; label: string }> = [
  { key: 'quota', label: 'keys.quotaAmount' },
  { key: 'rate_limit_5h', label: 'keys.rateLimit5h' },
  { key: 'rate_limit_1d', label: 'keys.rateLimit1d' },
  { key: 'rate_limit_7d', label: 'keys.rateLimit7d' }
]
const ipFields: Array<{ key: IPField; label: string }> = [
  { key: 'ip_whitelist', label: 'keys.ipWhitelist' },
  { key: 'ip_blacklist', label: 'keys.ipBlacklist' }
]
const groupOptions = computed(() => props.groups.map((group) => ({ value: group.id, label: group.name })))
const statusOptions = computed(() => [
  { value: 'active', label: t('keys.enable') },
  { value: 'inactive', label: t('keys.disable') }
])

const validationError = computed(() => {
  if (enabled.group_id && !props.groups.some((group) => group.id === groupId.value)) {
    return t('keys.groupRequired')
  }
  for (const { key } of limitFields) {
    if (!enabled[key]) continue
    const value = String(limits[key]).trim()
    if (!value || !Number.isFinite(Number(value)) || Number(value) < 0) {
      return t('keys.bulkEdit.invalidLimit')
    }
  }
  if (enabled.expires_at && !neverExpires.value && !Number.isFinite(Date.parse(expirationDate.value))) {
    return t('keys.bulkEdit.invalidExpiration')
  }
  return ''
})
const canSubmit = computed(() =>
  pendingKeys.value.length > 0 && Object.values(enabled).some(Boolean)
  && !validationError.value && !submitting.value
)

watch(() => props.show, (show) => {
  if (!show) return
  pendingKeys.value = props.selectedKeys.map(({ id, name }) => ({ id, name }))
  failures.value = []
  for (const field of Object.keys(enabled) as EditableField[]) enabled[field] = false
  for (const { key } of limitFields) limits[key] = ''
  for (const { key } of ipFields) ipLists[key] = ''
  groupId.value = null
  status.value = 'active'
  neverExpires.value = false
  expirationDate.value = ''
}, { immediate: true })

const close = () => {
  if (!submitting.value) emit('close')
}

const errorMessage = (error: unknown): string => {
  const message = (error as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : t('keys.failedToSave')
}

const submit = async () => {
  if (!canSubmit.value) return
  const updates: UpdateApiKeyRequest = {}
  if (enabled.group_id) updates.group_id = groupId.value
  if (enabled.status) updates.status = status.value
  for (const { key } of limitFields) {
    if (enabled[key]) updates[key] = Number(limits[key])
  }
  for (const { key } of ipFields) {
    if (enabled[key]) updates[key] = ipLists[key].split('\n').map((ip) => ip.trim()).filter(Boolean)
  }
  if (enabled.expires_at) {
    updates.expires_at = neverExpires.value ? '' : new Date(expirationDate.value).toISOString()
  }

  submitting.value = true
  try {
    const result = await keysAPI.bulkUpdate(pendingKeys.value.map((key) => key.id), updates)
    failures.value = result.failures.map(({ id, error }) => ({
      id,
      name: pendingKeys.value.find((key) => key.id === id)?.name ?? '',
      message: errorMessage(error)
    }))
    const failedIds = new Set(result.failures.map(({ id }) => id))
    pendingKeys.value = pendingKeys.value.filter((key) => failedIds.has(key.id))
    if (result.succeededIds.length) emit('updated', result.succeededIds)
    if (result.failures.length) {
      appStore.showError(t('keys.bulkEdit.partialFailure', {
        success: result.succeededIds.length, failed: result.failures.length
      }))
    } else {
      appStore.showSuccess(t('keys.bulkEdit.success', { count: result.succeededIds.length }))
      emit('close')
    }
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    submitting.value = false
  }
}
</script>
