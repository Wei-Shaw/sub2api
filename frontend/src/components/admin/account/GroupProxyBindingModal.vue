<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.groupProxyBinding.title')"
    width="narrow"
    close-on-click-outside
    @close="handleClose"
  >
    <div class="space-y-5">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.accounts.groupProxyBinding.description') }}
      </p>

      <div>
        <label class="input-label" for="group-proxy-binding-group">
          {{ t('admin.accounts.groupProxyBinding.group') }}
        </label>
        <Select
          id="group-proxy-binding-group"
          v-model="selectedGroupId"
          :options="groupOptions"
          :placeholder="t('admin.accounts.groupProxyBinding.selectGroup')"
          searchable
          :disabled="submitting"
        />
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.accounts.groupProxyBinding.proxy') }}
        </label>
        <ProxySelector
          v-model="selectedProxyId"
          :proxies="proxies"
          :allow-no-proxy="false"
          :disabled="submitting"
        />
      </div>

      <div
        v-if="selectedGroup"
        class="flex items-center justify-between rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800"
      >
        <div class="flex min-w-0 items-center gap-3">
          <Icon name="users" size="md" class="flex-shrink-0 text-primary-500" />
          <div class="min-w-0">
            <div class="truncate text-sm font-medium text-gray-900 dark:text-white">
              {{ selectedGroup.name }}
            </div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ selectedGroup.platform }}
            </div>
          </div>
        </div>
        <span class="ml-3 flex-shrink-0 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.groupProxyBinding.accountCount', { count: selectedAccountCount }) }}
        </span>
      </div>

      <div
        v-if="selectedGroup && selectedProxy"
        class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-700/50 dark:bg-amber-900/20 dark:text-amber-200"
      >
        {{
          t('admin.accounts.groupProxyBinding.overwriteWarning', {
            count: selectedAccountCount,
            proxy: selectedProxy.name
          })
        }}
      </div>
    </div>

    <template #footer>
      <div class="flex w-full justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          data-testid="bind-group-proxy-submit"
          type="button"
          class="btn btn-primary"
          :disabled="!canSubmit"
          @click="handleSubmit"
        >
          <Icon name="link" size="sm" class="mr-2" />
          {{
            submitting
              ? t('admin.accounts.groupProxyBinding.binding')
              : t('admin.accounts.groupProxyBinding.confirm')
          }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminGroup, Proxy } from '@/types'
import type { BulkAccountUpdateResult } from '@/api/admin/accounts'

interface Props {
  show: boolean
  groups: AdminGroup[]
  proxies: Proxy[]
}

interface Emits {
  (e: 'close'): void
  (e: 'updated', result: BulkAccountUpdateResult): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()
const appStore = useAppStore()

const selectedGroupId = ref<number | null>(null)
const selectedProxyId = ref<number | null>(null)
const submitting = ref(false)

const sortedGroups = computed(() =>
  [...props.groups].sort((left, right) => left.sort_order - right.sort_order || left.name.localeCompare(right.name))
)
const groupOptions = computed<SelectOption[]>(() =>
  sortedGroups.value.map(group => ({
    value: group.id,
    label: `${group.name} (${group.platform}, ${group.account_count || 0})`
  }))
)
const selectedGroup = computed(() => props.groups.find(group => group.id === selectedGroupId.value) || null)
const selectedProxy = computed(() => props.proxies.find(proxy => proxy.id === selectedProxyId.value) || null)
const selectedAccountCount = computed(() => selectedGroup.value?.account_count || 0)
const canSubmit = computed(
  () => !submitting.value && selectedGroup.value !== null && selectedProxy.value !== null && selectedAccountCount.value > 0
)

watch(
  () => props.show,
  (show) => {
    if (!show) return
    selectedGroupId.value = null
    selectedProxyId.value = null
  }
)

const handleSubmit = async () => {
  if (!canSubmit.value || selectedGroupId.value === null || selectedProxyId.value === null) return
  submitting.value = true
  try {
    const result = await adminAPI.accounts.bindProxyByGroup(selectedGroupId.value, selectedProxyId.value)
    if (result.failed > 0) {
      appStore.showWarning(
        t('admin.accounts.groupProxyBinding.partialSuccess', {
          success: result.success,
          failed: result.failed
        })
      )
    } else {
      appStore.showSuccess(t('admin.accounts.groupProxyBinding.success', { count: result.success }))
    }
    emit('updated', result)
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(error, t('admin.accounts.groupProxyBinding.failed'))
    )
  } finally {
    submitting.value = false
  }
}

const handleClose = () => {
  if (!submitting.value) emit('close')
}
</script>
