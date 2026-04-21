<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.channelMonitor.editTitle') : t('admin.channelMonitor.createTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <form id="channel-monitor-form" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.name') REDACTEDREDACTED <span class="text-red-500">*</span></label>
        <input v-model="form.name" type="text" required class="input" :placeholder="t('admin.channelMonitor.form.namePlaceholder')" />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.provider') REDACTEDREDACTED <span class="text-red-500">*</span></label>
        <div class="grid grid-cols-3 gap-3">
          <button
            v-for="opt in providerOptions"
            :key="opt.value"
            type="button"
            :aria-pressed="form.provider === opt.value"
            class="flex items-center justify-center gap-2 rounded-lg border-2 px-3 py-2.5 text-sm font-medium transition-colors"
            :class="providerPickerClass(opt.value, form.provider === opt.value)"
            @click="form.provider = opt.value"
          >
            <ProviderIcon :provider="opt.value" :size="18" />
            <span>{{ opt.label REDACTEDREDACTED</span>
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.endpoint') REDACTEDREDACTED <span class="text-red-500">*</span></label>
        <div class="flex gap-2">
          <input v-model="form.endpoint" type="text" required class="input flex-1" :placeholder="t('admin.channelMonitor.form.endpointPlaceholder')" />
          <button type="button" @click="useCurrentDomain" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useCurrentDomain') REDACTEDREDACTED
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.apiKey') REDACTEDREDACTED<span v-if="!editing" class="text-red-500"> *</span>
        </label>
        <div class="flex gap-2">
          <input
            v-model="form.api_key"
            type="password"
            :required="!editing"
            class="input flex-1"
            :placeholder="editing ? t('admin.channelMonitor.form.apiKeyEditPlaceholder') : t('admin.channelMonitor.form.apiKeyPlaceholder')"
          />
          <button type="button" @click="openMyKeyPicker" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useMyKey') REDACTEDREDACTED
          </button>
        </div>
        <p v-if="editing && editing.api_key_masked" class="mt-1 text-xs text-gray-400">{{ editing.api_key_masked REDACTEDREDACTED</p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.primaryModel') REDACTEDREDACTED <span class="text-red-500">*</span></label>
        <input
          v-model="form.primary_model"
          type="text"
          required
          class="input font-medium"
          :class="getPlatformTextClass(form.provider)"
          :placeholder="t('admin.channelMonitor.form.primaryModelPlaceholder')"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.extraModels') REDACTEDREDACTED</label>
        <ModelTagInput
          :models="form.extra_models"
          :platform="form.provider"
          :placeholder="t('admin.channelMonitor.form.extraModelsPlaceholder')"
          @update:models="form.extra_models = $event"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.groupName') REDACTEDREDACTED</label>
        <input v-model="form.group_name" type="text" class="input" :placeholder="t('admin.channelMonitor.form.groupNamePlaceholder')" />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.intervalSeconds') REDACTEDREDACTED <span class="text-red-500">*</span></label>
        <input v-model.number="form.interval_seconds" type="number" min="15" max="3600" required class="input" />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.intervalSecondsHint') REDACTEDREDACTED</p>
      </div>

      <div class="flex items-center justify-between">
        <label class="input-label mb-0">{{ t('admin.channelMonitor.form.enabled') REDACTEDREDACTED</label>
        <Toggle v-model="form.enabled" />
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button
          type="submit"
          form="channel-monitor-form"
          :disabled="submitting"
          class="btn btn-primary"
        >
          {{ submitting
            ? t('common.submitting')
            : editing ? t('common.update') : t('common.create') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>

  <MonitorKeyPickerDialog
    :show="showKeyPicker"
    :loading="myKeysLoading"
    :keys="myActiveKeys"
    :provider="form.provider"
    :user-group-rates="userGroupRates"
    @close="showKeyPicker = false"
    @pick="pickMyKey"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import { adminAPI REDACTED from '@/api/admin'
import { keysAPI REDACTED from '@/api/keys'
import { userGroupsAPI REDACTED from '@/api/groups'
import type {
  ChannelMonitor,
  CreateParams,
  Provider,
  UpdateParams,
REDACTED from '@/api/admin/channelMonitor'
import type { ApiKey REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import ModelTagInput from '@/components/admin/channel/ModelTagInput.vue'
import { getPlatformTextClass REDACTED from '@/components/admin/channel/types'
import MonitorKeyPickerDialog from '@/components/admin/monitor/MonitorKeyPickerDialog.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import { useChannelMonitorFormat REDACTED from '@/composables/useChannelMonitorFormat'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
  DEFAULT_INTERVAL_SECONDS,
REDACTED from '@/constants/channelMonitor'

const props = defineProps<{
  show: boolean
  monitor: ChannelMonitor | null
REDACTED>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()
const { providerPickerClass REDACTED = useChannelMonitorFormat()

// System-configured default interval for new monitors. Falls back to the static
// constant when public settings haven't loaded yet or store the legacy 0 value.
const systemDefaultInterval = computed<number>(() => {
  const configured = appStore.cachedPublicSettings?.channel_monitor_default_interval_seconds
  return configured && configured > 0 ? configured : DEFAULT_INTERVAL_SECONDS
REDACTED)

// editing is true when we have an existing monitor
const editing = computed<ChannelMonitor | null>(() => props.monitor)

const submitting = ref(false)

// API key picker
const showKeyPicker = ref(false)
const myKeysLoading = ref(false)
const myActiveKeys = ref<ApiKey[]>([])
const userGroupRates = ref<Record<number, number>>({REDACTED)

interface MonitorForm {
  name: string
  provider: Provider
  endpoint: string
  api_key: string
  primary_model: string
  extra_models: string[]
  group_name: string
  interval_seconds: number
  enabled: boolean
REDACTED

const form = reactive<MonitorForm>({
  name: '',
  provider: PROVIDER_OPENAI,
  endpoint: '',
  api_key: '',
  primary_model: '',
  extra_models: [],
  group_name: '',
  interval_seconds: systemDefaultInterval.value,
  enabled: true,
REDACTED)

interface ProviderOption {
  value: Provider
  label: string
REDACTED

const providerOptions = computed<ProviderOption[]>(() => [
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') REDACTED,
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') REDACTED,
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') REDACTED,
])

// Clear api_key whenever provider changes to avoid cross-provider key mismatch.
// Editing mode loads api_key='' via loadFromMonitor and only sets it on user
// typing, so clearing on provider change is always a safe no-op until the user
// picks a new key.
watch(() => form.provider, () => {
  form.api_key = ''
REDACTED)

function resetForm() {
  form.name = ''
  form.provider = PROVIDER_OPENAI
  form.endpoint = ''
  form.api_key = ''
  form.primary_model = ''
  form.extra_models = []
  form.group_name = ''
  form.interval_seconds = systemDefaultInterval.value
  form.enabled = true
REDACTED

function loadFromMonitor(m: ChannelMonitor) {
  form.name = m.name
  form.provider = m.provider
  form.endpoint = m.endpoint
  form.api_key = ''
  form.primary_model = m.primary_model
  form.extra_models = [...(m.extra_models || [])]
  form.group_name = m.group_name || ''
  form.interval_seconds = m.interval_seconds || systemDefaultInterval.value
  form.enabled = m.enabled
REDACTED

// Re-sync form whenever the dialog is opened or the target monitor changes.
watch(
  () => [props.show, props.monitor] as const,
  ([show, m]) => {
    if (!show) return
    if (m) loadFromMonitor(m)
    else resetForm()
  REDACTED,
  { immediate: true REDACTED,
)

function useCurrentDomain() {
  form.endpoint = window.location.origin
REDACTED

async function openMyKeyPicker() {
  showKeyPicker.value = true
  if (myActiveKeys.value.length > 0) return
  myKeysLoading.value = true
  try {
    const [res, rates] = await Promise.all([
      keysAPI.list(1, 100, { status: 'active' REDACTED),
      userGroupsAPI.getUserGroupRates(),
    ])
    const items = res.items || []
    const now = Date.now()
    myActiveKeys.value = items.filter(k => {
      if (k.status !== 'active') return false
      if (!k.expires_at) return true
      return new Date(k.expires_at).getTime() > now
    REDACTED)
    userGroupRates.value = rates
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.channelMonitor.form.noActiveKey')))
  REDACTED finally {
    myKeysLoading.value = false
  REDACTED
REDACTED

function pickMyKey(k: ApiKey) {
  form.api_key = k.key
  showKeyPicker.value = false
REDACTED

function buildPayload(): CreateParams {
  return {
    name: form.name.trim(),
    provider: form.provider,
    endpoint: form.endpoint.trim(),
    api_key: form.api_key.trim(),
    primary_model: form.primary_model.trim(),
    extra_models: form.extra_models,
    group_name: form.group_name.trim(),
    enabled: form.enabled,
    interval_seconds: form.interval_seconds,
  REDACTED
REDACTED

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    appStore.showError(t('admin.channelMonitor.nameRequired'))
    return
  REDACTED
  if (!form.primary_model.trim()) {
    appStore.showError(t('admin.channelMonitor.primaryModelRequired'))
    return
  REDACTED

  submitting.value = true
  try {
    const target = editing.value
    if (target) {
      const { api_key, ...rest REDACTED = buildPayload()
      const req: UpdateParams = rest
      // Only send api_key if user typed a new value
      if (api_key) req.api_key = api_key
      await adminAPI.channelMonitor.update(target.id, req)
      appStore.showSuccess(t('admin.channelMonitor.updateSuccess'))
    REDACTED else {
      await adminAPI.channelMonitor.create(buildPayload())
      appStore.showSuccess(t('admin.channelMonitor.createSuccess'))
    REDACTED
    emit('saved')
    emit('close')
  REDACTED catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED
</script>
