<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.channelMonitor.editTitle') : t('admin.channelMonitor.createTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <form id="channel-monitor-form" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.name') }} <span class="input-required">*</span></label>
        <input v-model="form.name" type="text" required class="input"
          :placeholder="t('admin.channelMonitor.form.namePlaceholder')" />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.provider') }} <span class="input-required">*</span></label>
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
            <PlatformIcon :platform="opt.value" size="sm" />
            <span>{{ opt.label }}</span>
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.endpoint') }} <span class="input-required">*</span></label>
        <div class="flex gap-2">
          <input v-model="form.endpoint" type="text" required class="input flex-1"
            :placeholder="t('admin.channelMonitor.form.endpointPlaceholder')" />
          <button type="button" @click="useCurrentDomain" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useCurrentDomain') }}
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.form.apiKey') }}<span v-if="!editing" class="input-required"> *</span>
        </label>
        <div class="flex gap-2">
          <input v-model="form.api_key" type="password" autocomplete="new-password"
            data-1p-ignore data-lpignore="true" data-bwignore="true" :required="!editing"
            class="input flex-1 font-mono"
            :placeholder="editing ? t('admin.channelMonitor.form.apiKeyEditPlaceholder') : t('admin.channelMonitor.form.apiKeyPlaceholder')" />
          <button type="button" @click="openMyKeyPicker" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useMyKey') }}
          </button>
        </div>
        <p v-if="editing && editing.api_key_masked" class="mt-1 text-xs text-gray-400">
          {{ editing.api_key_masked }}
        </p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.primaryModel') }} <span class="input-required">*</span></label>
        <input v-model="form.primary_model" type="text" required class="input font-medium"
          :class="getPlatformTextClass(form.provider)"
          :placeholder="t('admin.channelMonitor.form.primaryModelPlaceholder')" />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.extraModels') }}</label>
        <textarea v-model="extraModelsText" class="input min-h-[64px]"
          :placeholder="t('admin.channelMonitor.form.extraModelsPlaceholder')" />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.groupName') }}</label>
        <input v-model="form.group_name" type="text" class="input"
          :placeholder="t('admin.channelMonitor.form.groupNamePlaceholder')" />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.intervalSeconds') }} <span class="input-required">*</span></label>
        <input v-model.number="form.interval_seconds" type="number" min="15" max="3600" required class="input" />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.intervalSecondsHint') }}</p>
      </div>

      <div class="flex items-center justify-between">
        <label class="input-label mb-0">{{ t('admin.channelMonitor.form.enabled') }}</label>
        <Toggle v-model="form.enabled" />
      </div>

      <details class="rounded-lg border border-gray-200 bg-gray-50/50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
        <summary class="cursor-pointer select-none text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.channelMonitor.advanced.section') }}
        </summary>
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.advanced.sectionHint') }}</p>
        <div class="mt-4">
          <MonitorAdvancedRequestConfig
            :extra-headers="form.extra_headers"
            :body-override-mode="form.body_override_mode"
            :body-override="form.body_override"
            @update:extra-headers="form.extra_headers = $event"
            @update:body-override-mode="form.body_override_mode = $event"
            @update:body-override="form.body_override = $event"
          />
        </div>
      </details>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" form="channel-monitor-form" :disabled="submitting" class="btn btn-primary">
          {{ submitting ? t('common.submitting') : editing ? t('common.update') : t('common.create') }}
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
    @close="closeKeyPicker"
    @pick="pickMyKey"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, Toggle, PlatformIcon, platformTextClass } from '@sub2api/plugin-sdk'
import type { ChannelMonitor } from '../../../api/admin/channelMonitor'
import { useChannelMonitorFormat } from '../../../composables/useChannelMonitorFormat'
import { useMonitorKeyPicker } from '../../../composables/useMonitorKeyPicker'
import { useMonitorFormLogic } from '../../../composables/useMonitorFormLogic'
import MonitorAdvancedRequestConfig from './MonitorAdvancedRequestConfig.vue'
import MonitorKeyPickerDialog from './MonitorKeyPickerDialog.vue'

const props = defineProps<{
  show: boolean
  monitor: ChannelMonitor | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const { providerPickerClass } = useChannelMonitorFormat()
const getPlatformTextClass = platformTextClass

const { form, extraModelsText, submitting, editing, useCurrentDomain, handleSubmit } =
  useMonitorFormLogic(props, { saved: () => emit('saved'), close: () => emit('close') })

const {
  showKeyPicker, myKeysLoading, myActiveKeys, userGroupRates,
  openMyKeyPicker, pickMyKey, closeKeyPicker,
} = useMonitorKeyPicker((key) => { form.value.api_key = key })

const providerOptions = computed<{ value: string; label: string }[]>(() => [
  { value: 'anthropic', label: t('monitorCommon.providers.anthropic') },
  { value: 'openai', label: t('monitorCommon.providers.openai') },
  { value: 'gemini', label: t('monitorCommon.providers.gemini') },
])
</script>
