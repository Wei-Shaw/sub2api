<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.template.managerTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <!-- provider tabs -->
    <div class="mb-4 border-b border-gray-200 dark:border-dark-700">
      <div role="tablist" class="flex gap-1">
        <button
          v-for="tab in providerTabs"
          :key="tab.value"
          type="button"
          role="tab"
          :aria-selected="activeProvider === tab.value"
          class="px-4 py-2 text-sm font-medium transition-colors"
          :class="tabClass(tab.value)"
          @click="activeProvider = tab.value"
        >
          {{ tab.label }}
          <span
            v-if="countByProvider[tab.value] > 0"
            class="ml-1.5 rounded-full bg-gray-100 px-2 py-0.5 text-xs dark:bg-dark-700"
          >
            {{ countByProvider[tab.value] }}
          </span>
        </button>
      </div>
    </div>

    <!-- active provider list -->
    <MonitorTemplateList
      v-if="!editing"
      :templates="templatesForActiveProvider"
      :loading="loading"
      @create="openCreateForm"
      @edit="openEditForm"
      @delete="handleDelete"
      @apply="confirmApply"
    />

    <!-- edit / create form -->
    <div v-else class="space-y-4">
      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.template.form.name') }}
          <span class="input-required">*</span>
        </label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.channelMonitor.template.form.namePlaceholder')"
        />
      </div>

      <div v-if="editing === 'new'">
        <label class="input-label">
          {{ t('admin.channelMonitor.form.provider') }}
          <span class="input-required">*</span>
        </label>
        <div class="grid grid-cols-3 gap-3">
          <button
            v-for="opt in providerTabs"
            :key="opt.value"
            type="button"
            class="rounded-lg border-2 px-3 py-2 text-sm font-medium transition-colors"
            :class="providerPickerClass(opt.value, form.provider === opt.value)"
            @click="form.provider = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.channelMonitor.template.form.description') }}
        </label>
        <input
          v-model="form.description"
          type="text"
          class="input"
          :placeholder="t('admin.channelMonitor.template.form.descriptionPlaceholder')"
        />
      </div>

      <MonitorAdvancedRequestConfig
        :extra-headers="form.extra_headers"
        :body-override-mode="form.body_override_mode"
        :body-override="form.body_override"
        @update:extra-headers="form.extra_headers = $event"
        @update:body-override-mode="form.body_override_mode = $event"
        @update:body-override="form.body_override = $event"
      />
    </div>

    <template #footer>
      <div class="flex w-full items-center justify-between">
        <div>
          <button v-if="editing" class="btn btn-secondary" @click="backToList">
            {{ t('common.back') }}
          </button>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-secondary" @click="$emit('close')">
            {{ t('common.close') }}
          </button>
          <button
            v-if="editing"
            class="btn btn-primary"
            :disabled="submitting"
            @click="handleSubmit"
          >
            {{
              submitting
                ? t('common.submitting')
                : editing === 'new'
                  ? t('common.create')
                  : t('common.update')
            }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>

  <MonitorTemplateApplyPickerDialog
    :show="applyPicker.show"
    :template-id="applyPicker.tpl ? applyPicker.tpl.id : null"
    :template-name="applyPicker.tpl ? applyPicker.tpl.name : ''"
    @close="applyPicker.show = false"
    @applied="onApplied"
  />

  <ConfirmDialog
    :show="confirmDelete.show"
    :title="t('common.delete')"
    :message="confirmDeleteMessage"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="doDelete"
    @cancel="confirmDelete.show = false"
  />
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { BaseDialog, ConfirmDialog } from '@sub2api/plugin-sdk'
import MonitorAdvancedRequestConfig from './MonitorAdvancedRequestConfig.vue'
import MonitorTemplateApplyPickerDialog from './MonitorTemplateApplyPickerDialog.vue'
import MonitorTemplateList from './MonitorTemplateList.vue'
import { useChannelMonitorFormat } from '../../../composables/useChannelMonitorFormat'
import { useTemplateManager } from '../../../composables/useTemplateManager'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated'): void
}>()

const { t } = useI18n()
const { providerPickerClass } = useChannelMonitorFormat()

const {
  activeProvider,
  providerTabs,
  templatesForActiveProvider,
  countByProvider,
  loading,
  editing,
  submitting,
  form,
  applyPicker,
  confirmDelete,
  confirmDeleteMessage,
  openCreateForm,
  openEditForm,
  backToList,
  handleSubmit,
  confirmApply,
  onApplied,
  handleDelete,
  doDelete,
  tabClass,
} = useTemplateManager(props, { updated: () => emit('updated') })
</script>
