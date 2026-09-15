<template>
  <BaseDialog
    :show="show"
    :title="t('admin.groups.bulkEdit.title')"
    width="normal"
    :close-on-escape="!submitting"
    :show-close-button="!submitting"
    @close="close"
  >
    <form id="bulk-edit-groups-form" class="space-y-5" @submit.prevent="submit">
      <div class="space-y-1 text-sm">
        <p class="font-medium text-gray-900 dark:text-white">
          {{ t('admin.groups.bulkEdit.selectedCount', { count: pendingGroups.length }) }}
        </p>
        <p class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.bulkEdit.hint') }}</p>
      </div>

      <fieldset :disabled="submitting" class="space-y-5">
        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.rate_multiplier" type="checkbox" class="checkbox" data-test="enable-rate" />
            {{ t('admin.groups.form.rateMultiplierLabel') }}
          </label>
          <div v-if="enabled.rate_multiplier">
            <input
              v-model="rateMultiplier"
              type="number"
              min="0"
              step="any"
              required
              class="input"
              :aria-label="t('admin.groups.form.rateMultiplierLabel')"
              data-test="rate-input"
            />
            <p class="input-hint">{{ t('admin.groups.form.rateMultiplierHint') }}</p>
          </div>
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.status" type="checkbox" class="checkbox" data-test="enable-status" />
            {{ t('admin.groups.form.statusLabel') }}
          </label>
          <Select
            v-if="enabled.status"
            v-model="status"
            :options="statusOptions"
            :disabled="submitting"
            :aria-label="t('admin.groups.form.statusLabel')"
            data-test="status-input"
          />
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.is_exclusive" type="checkbox" class="checkbox" data-test="enable-exclusive" />
            {{ t('admin.groups.form.exclusiveLabel') }}
          </label>
          <Select
            v-if="enabled.is_exclusive"
            v-model="isExclusive"
            :options="exclusiveOptions"
            :disabled="submitting"
            :aria-label="t('admin.groups.form.exclusiveLabel')"
            data-test="exclusive-input"
          />
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.description" type="checkbox" class="checkbox" data-test="enable-description" />
            {{ t('admin.groups.form.descriptionLabel') }}
          </label>
          <div v-if="enabled.description">
            <textarea
              v-model="description"
              rows="3"
              class="input"
              :aria-label="t('admin.groups.form.descriptionLabel')"
              data-test="description-input"
            />
            <p class="input-hint">{{ t('admin.groups.bulkEdit.descriptionHint') }}</p>
          </div>
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-2 text-sm font-medium">
            <input v-model="enabled.rpm_limit" type="checkbox" class="checkbox" data-test="enable-rpm" />
            {{ t('admin.groups.form.rpmLimit') }}
          </label>
          <div v-if="enabled.rpm_limit">
            <input
              v-model="rpmLimit"
              type="number"
              min="0"
              step="1"
              required
              class="input"
              :aria-label="t('admin.groups.form.rpmLimit')"
              data-test="rpm-input"
            />
            <p class="input-hint">{{ t('admin.groups.form.rpmLimitHint') }}</p>
          </div>
        </div>

        <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-700">
          <p v-if="!allSubscriptions" class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.groups.bulkEdit.subscriptionOnly') }}
          </p>
          <div v-for="field in limitFields" :key="field.key" class="space-y-2">
            <label class="flex items-center gap-2 text-sm font-medium" :class="{ 'opacity-50': !allSubscriptions }">
              <input
                v-model="enabled[field.key]"
                type="checkbox"
                class="checkbox"
                :disabled="!allSubscriptions"
                :data-test="`enable-${field.key}`"
              />
              {{ t(field.label) }}
            </label>
            <div v-if="enabled[field.key]" class="space-y-2">
              <label class="flex items-center gap-2 text-sm">
                <input v-model="unlimited[field.key]" type="checkbox" class="checkbox" :data-test="`unlimited-${field.key}`" />
                {{ t('admin.groups.subscription.noLimit') }}
              </label>
              <input
                v-if="!unlimited[field.key]"
                v-model="limits[field.key]"
                type="number"
                min="0"
                step="any"
                required
                class="input"
                :aria-label="t(field.label)"
                :data-test="`${field.key}-input`"
              />
              <p class="input-hint">{{ t('admin.groups.bulkEdit.limitHint') }}</p>
            </div>
          </div>
        </div>
      </fieldset>

      <p v-if="validationError" role="alert" class="text-sm text-red-600 dark:text-red-400">
        {{ validationError }}
      </p>
      <div v-if="failures.length" role="alert" class="space-y-2 text-sm text-red-600 dark:text-red-400">
        <p>{{ t('admin.groups.bulkEdit.failureHint') }}</p>
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
        form="bulk-edit-groups-form"
        class="btn btn-primary"
        :disabled="!canSubmit"
        data-test="submit"
      >
        {{ submitting ? t('common.saving') : t('admin.groups.bulkEdit.apply', { count: pendingGroups.length }) }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { AdminGroup, UpdateGroupRequest } from '@/types'

type SelectedGroup = Pick<AdminGroup, 'id' | 'name' | 'subscription_type'>
type LimitField = 'daily_limit_usd' | 'weekly_limit_usd' | 'monthly_limit_usd'
type EditableField = LimitField | 'rate_multiplier' | 'status' | 'is_exclusive' | 'description' | 'rpm_limit'

const props = defineProps<{ show: boolean; selectedGroups: SelectedGroup[] }>()
const emit = defineEmits<{ close: []; updated: [succeededIds: number[]] }>()
const { t } = useI18n()
const appStore = useAppStore()
const submitting = ref(false)
const pendingGroups = ref<SelectedGroup[]>([])
const failures = ref<Array<{ id: number; name: string; message: string }>>([])
const enabled = reactive<Record<EditableField, boolean>>({
  rate_multiplier: false, status: false, is_exclusive: false, description: false,
  rpm_limit: false, daily_limit_usd: false, weekly_limit_usd: false, monthly_limit_usd: false
})
const rateMultiplier = ref<string | number>('')
const rpmLimit = ref<string | number>('')
const status = ref<'active' | 'inactive'>('active')
const isExclusive = ref(false)
const description = ref('')
const limits = reactive<Record<LimitField, string | number>>({ daily_limit_usd: '', weekly_limit_usd: '', monthly_limit_usd: '' })
const unlimited = reactive<Record<LimitField, boolean>>({ daily_limit_usd: false, weekly_limit_usd: false, monthly_limit_usd: false })
const limitFields: Array<{ key: LimitField; label: string }> = [
  { key: 'daily_limit_usd', label: 'admin.groups.subscription.dailyLimit' },
  { key: 'weekly_limit_usd', label: 'admin.groups.subscription.weeklyLimit' },
  { key: 'monthly_limit_usd', label: 'admin.groups.subscription.monthlyLimit' }
]
const allSubscriptions = computed(() => pendingGroups.value.length > 0
  && pendingGroups.value.every((group) => group.subscription_type === 'subscription'))
const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])
const exclusiveOptions = computed(() => [
  { value: true, label: t('admin.groups.exclusiveObj.yes') },
  { value: false, label: t('admin.groups.exclusiveObj.no') }
])

const parseNumber = (value: string | number) => String(value).trim() === '' ? NaN : Number(value)
const validationError = computed(() => {
  const rate = parseNumber(rateMultiplier.value)
  if (enabled.rate_multiplier && (!Number.isFinite(rate) || rate <= 0)) return t('admin.groups.bulkEdit.invalidRate')
  const rpm = parseNumber(rpmLimit.value)
  if (enabled.rpm_limit && (!Number.isSafeInteger(rpm) || rpm < 0)) return t('admin.groups.bulkEdit.invalidRPM')
  for (const { key } of limitFields) {
    if (!enabled[key]) continue
    if (!allSubscriptions.value) return t('admin.groups.bulkEdit.subscriptionOnly')
    const value = parseNumber(limits[key])
    if (!unlimited[key] && (!Number.isFinite(value) || value < 0)) return t('admin.groups.bulkEdit.invalidLimit')
  }
  return ''
})
const canSubmit = computed(() => pendingGroups.value.length > 0 && Object.values(enabled).some(Boolean)
  && !validationError.value && !submitting.value)

watch(() => props.show, (show) => {
  if (!show) return
  pendingGroups.value = props.selectedGroups.map(({ id, name, subscription_type }) => ({ id, name, subscription_type }))
  failures.value = []
  for (const field of Object.keys(enabled) as EditableField[]) enabled[field] = false
  for (const { key } of limitFields) {
    limits[key] = ''
    unlimited[key] = false
  }
  rateMultiplier.value = ''
  rpmLimit.value = ''
  status.value = 'active'
  isExclusive.value = false
  description.value = ''
}, { immediate: true })

const close = () => {
  if (!submitting.value) emit('close')
}
const errorMessage = (error: unknown): string => {
  const message = (error as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : t('admin.groups.failedToUpdate')
}

const submit = async () => {
  if (!canSubmit.value) return
  const updates: UpdateGroupRequest = {}
  if (enabled.rate_multiplier) updates.rate_multiplier = Number(rateMultiplier.value)
  if (enabled.status) updates.status = status.value
  if (enabled.is_exclusive) updates.is_exclusive = isExclusive.value
  if (enabled.description) updates.description = description.value
  if (enabled.rpm_limit) updates.rpm_limit = Number(rpmLimit.value)
  for (const { key } of limitFields) {
    if (enabled[key]) updates[key] = unlimited[key] ? null : Number(limits[key])
  }

  submitting.value = true
  try {
    const result = await adminAPI.groups.bulkUpdate(pendingGroups.value.map((group) => group.id), updates)
    failures.value = result.failures.map(({ id, error }) => ({
      id, name: pendingGroups.value.find((group) => group.id === id)?.name ?? '', message: errorMessage(error)
    }))
    const failedIds = new Set(result.failures.map(({ id }) => id))
    pendingGroups.value = pendingGroups.value.filter((group) => failedIds.has(group.id))
    if (result.succeededIds.length) emit('updated', result.succeededIds)
    if (result.failures.length) {
      appStore.showError(t('admin.groups.bulkEdit.partialFailure', {
        success: result.succeededIds.length, failed: result.failures.length
      }))
    } else {
      appStore.showSuccess(t('admin.groups.bulkEdit.success', { count: result.succeededIds.length }))
      emit('close')
    }
  } catch (error) {
    appStore.showError(errorMessage(error))
  } finally {
    submitting.value = false
  }
}
</script>
