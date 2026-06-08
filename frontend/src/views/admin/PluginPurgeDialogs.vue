<template>
  <!-- Remove files confirm dialog -->
  <ConfirmDialog
    :show="!!removeFilesTarget"
    :title="removeFilesTarget ? t('admin.plugins.removeFilesConfirm.title', { name: removeFilesTarget.display_name || removeFilesTarget.name }) : ''"
    :message="t('admin.plugins.removeFilesConfirm.message')"
    :confirm-text="t('admin.plugins.removeFilesConfirm.confirmButton')"
    :danger="true"
    @confirm="$emit('confirm-remove-files')"
    @cancel="$emit('cancel-remove-files')"
  />

  <!-- Soft uninstall confirm -->
  <ConfirmDialog
    :show="!!uninstallTarget"
    :title="uninstallTarget ? t('admin.plugins.uninstallSoftConfirm.title', { name: uninstallTarget.display_name || uninstallTarget.name }) : ''"
    :message="t('admin.plugins.uninstallSoftConfirm.message')"
    :confirm-text="t('admin.plugins.uninstallButton')"
    :danger="true"
    @confirm="$emit('confirm-uninstall')"
    @cancel="$emit('cancel-uninstall')"
  />

  <!-- Hard purge dialog -->
  <BaseDialog
    v-if="purgeTarget"
    :show="!!purgeTarget"
    :title="t('admin.plugins.purgeConfirm.title', { name: purgeTarget.display_name || purgeTarget.name })"
    width="narrow"
    @close="$emit('cancel-purge')"
  >
    <div class="space-y-3">
      <div class="rounded-md border border-red-300 bg-red-50 p-3 dark:border-red-700 dark:bg-red-900/30">
        <div class="flex items-center gap-2 text-sm font-semibold text-red-800 dark:text-red-200">
          <Icon name="exclamationCircle" size="sm" />
          {{ t('admin.plugins.purgeConfirm.warning') }}
        </div>
      </div>

      <ul class="space-y-1.5 text-xs text-gray-700 dark:text-gray-300">
        <li class="flex items-start gap-2">
          <Icon name="trash" size="xs" class="mt-0.5 shrink-0 text-red-500" />
          <span>{{ t('admin.plugins.purgeConfirm.willLose') }}</span>
        </li>
        <li class="flex items-start gap-2">
          <Icon name="checkCircle" size="xs" class="mt-0.5 shrink-0 text-emerald-500" />
          <span>{{ t('admin.plugins.purgeConfirm.willKeep') }}</span>
        </li>
        <li class="flex items-start gap-2">
          <Icon name="exclamationCircle" size="xs" class="mt-0.5 shrink-0 text-amber-500" />
          <span>{{ t('admin.plugins.purgeConfirm.irreversibleWarning') }}</span>
        </li>
      </ul>

      <div>
        <label class="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.plugins.purgeConfirm.typeName', { name: purgeTarget.name }) }}
        </label>
        <input
          :value="purgeNameInput"
          type="text"
          class="w-full rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-mono text-gray-900 focus:border-red-500 focus:outline-none focus:ring-1 focus:ring-red-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100"
          :placeholder="purgeTarget.name"
          autocomplete="off"
          spellcheck="false"
          @input="$emit('update:purgeNameInput', ($event.target as HTMLInputElement).value)"
        />
      </div>

      <div
        v-if="purgeError"
        class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300"
      >
        {{ purgeError }}
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          type="button"
          class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
          @click="$emit('cancel-purge')"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="!purgeNameMatches || purgeBusy"
          @click="$emit('confirm-purge')"
        >
          {{ t('admin.plugins.purgeConfirm.confirmButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, ConfirmDialog, Icon } from '@sub2api/plugin-sdk'
import type { PluginInfo } from './pluginHelpers'

const props = defineProps<{
  removeFilesTarget: PluginInfo | null
  uninstallTarget: PluginInfo | null
  purgeTarget: PluginInfo | null
  purgeNameInput: string
  purgeError: string
  purgeBusy: boolean
}>()

defineEmits<{
  'confirm-remove-files': []
  'cancel-remove-files': []
  'confirm-uninstall': []
  'cancel-uninstall': []
  'confirm-purge': []
  'cancel-purge': []
  'update:purgeNameInput': [value: string]
}>()

const { t } = useI18n()

const purgeNameMatches = computed<boolean>(
  () => !!props.purgeTarget && props.purgeNameInput === props.purgeTarget.name
)
</script>
