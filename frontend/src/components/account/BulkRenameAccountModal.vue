<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.bulkRename.title')"
    width="wide"
    :close-on-escape="!applying"
    :show-close-button="!applying"
    @close="close"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('admin.accounts.bulkRename.hint') }}</p>
      <p class="text-sm font-medium">{{ t('admin.accounts.bulkRename.selection', { count: accountIds.length }) }}</p>
      <fieldset :disabled="applying" class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <label class="block text-sm">
          {{ t('admin.accounts.bulkRename.prefix') }}
          <input v-model="rule.prefix" data-testid="rename-prefix" type="text" maxlength="100" class="input mt-1 w-full" />
        </label>
        <label class="block text-sm">
          {{ t('admin.accounts.bulkRename.suffix') }}
          <input v-model="rule.suffix" data-testid="rename-suffix" type="text" maxlength="100" class="input mt-1 w-full" />
        </label>
        <label class="block text-sm">
          {{ t('admin.accounts.bulkRename.start') }}
          <input v-model.number="rule.start_number" data-testid="rename-start" type="number" min="0" max="9007199254740991" step="1" class="input mt-1 w-full" />
        </label>
        <label class="block text-sm">
          {{ t('admin.accounts.bulkRename.padding') }}
          <input v-model.number="rule.padding" data-testid="rename-padding" type="number" min="1" max="16" step="1" class="input mt-1 w-full" />
        </label>
        <label class="flex items-center gap-2 text-sm sm:col-span-2">
          <input v-model="rule.continue_sequence" type="checkbox" class="h-4 w-4 rounded" />
          {{ t('admin.accounts.bulkRename.continue') }}
        </label>
        <p v-if="rule.continue_sequence" class="text-xs text-gray-500 sm:col-span-2">{{ t('admin.accounts.bulkRename.continueHint') }}</p>
      </fieldset>
      <p v-if="!validSelection" role="alert" class="text-sm text-red-600">{{ t('admin.accounts.bulkRename.limit') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <template v-if="preview">
        <p class="text-sm text-gray-500">{{ t('admin.accounts.bulkRename.previewHint') }}</p>
        <div class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="w-full text-left text-sm">
            <thead class="sticky top-0 bg-gray-50 dark:bg-dark-800">
              <tr><th class="p-3">ID</th><th class="p-3">{{ t('admin.accounts.bulkRename.oldName') }}</th><th class="p-3">{{ t('admin.accounts.bulkRename.newName') }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="item in preview" :key="item.account_id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="p-3">{{ item.account_id }}</td><td class="break-all p-3">{{ item.old_name }}</td><td class="break-all p-3 font-medium">{{ item.new_name }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </div>
    <template #footer>
      <button class="btn btn-secondary" :disabled="applying" @click="close">{{ t('common.cancel') }}</button>
      <button data-testid="rename-preview" class="btn btn-secondary" :disabled="!validSelection || previewing || applying" @click="loadPreview">{{ previewing ? t('common.loading') : t('admin.accounts.bulkRename.preview') }}</button>
      <button data-testid="rename-apply" class="btn btn-primary" :disabled="!preview || previewing || applying" @click="apply">{{ applying ? t('common.loading') : t('admin.accounts.bulkRename.apply') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import type { AccountRenameItem } from '@/api/admin/accounts'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; accountIds: number[] }>()
const emit = defineEmits<{ close: []; updated: [] }>()
const { t } = useI18n()
const app = useAppStore()
const rule = reactive({ prefix: '', suffix: '', start_number: 1, padding: 2, continue_sequence: false })
const preview = ref<AccountRenameItem[] | null>(null)
const previewing = ref(false)
const applying = ref(false)
const error = ref('')
const validSelection = computed(() => props.accountIds.length > 0 && props.accountIds.length <= 1000)
let revision = 0
let idempotencyKey = ''

function invalidate() {
  revision++
  preview.value = null
  previewing.value = false
  error.value = ''
  idempotencyKey = ''
}
watch(rule, invalidate, { flush: 'sync' })
watch(() => [props.show, ...props.accountIds], invalidate, { flush: 'sync' })

function close() {
  if (!applying.value) emit('close')
}

async function loadPreview() {
  if (!validSelection.value || applying.value) return
  invalidate()
  const requestRevision = revision
  previewing.value = true
  try {
    const result = await adminAPI.accounts.previewAccountRenames({ ...rule, account_ids: [...props.accountIds] })
    if (requestRevision !== revision || !props.show) return
    preview.value = result.items
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    idempotencyKey = `account-rename-${requestID}`
  } catch (cause) {
    if (requestRevision === revision) error.value = (cause as { message?: string }).message || t('admin.accounts.bulkRename.failed')
  } finally {
    if (requestRevision === revision) previewing.value = false
  }
}

async function apply() {
  if (!preview.value || applying.value) return
  applying.value = true
  error.value = ''
  try {
    const result = await adminAPI.accounts.applyAccountRenames(preview.value, idempotencyKey)
    app.showSuccess(t('admin.accounts.bulkRename.success', { count: result.applied }))
    emit('updated')
  } catch (cause) {
    // Preserve the exact plan and key on an ambiguous network failure for a safe retry.
    error.value = (cause as { message?: string }).message || t('admin.accounts.bulkRename.failed')
  } finally {
    applying.value = false
  }
}
</script>
