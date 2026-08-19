<template>
  <BaseDialog :show="show" :title="t('admin.users.bulkHiddenGroups.title')" width="wide" @close="emit('close')">
    <div class="space-y-4"><p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.users.bulkHiddenGroups.hint') }}</p><p class="text-sm">{{ t('admin.users.bulkHiddenGroups.selectedCount', { count: selectedIds.length }) }}</p><div class="grid gap-2 sm:grid-cols-2"><label v-for="g in groups" :key="g.id" class="flex items-center gap-3 rounded-lg border p-3"><input v-model="hiddenIds" type="checkbox" :value="g.id" /><span>{{ g.name }}</span><span class="text-xs text-gray-400">{{ g.is_exclusive ? t('admin.users.exclusiveLabel') : t('admin.users.publicLabel') }}</span></label></div><p v-if="tooLarge" class="text-sm text-red-600">{{ t('admin.users.bulkHiddenGroups.selectionLimit', { max: 500 }) }}</p></div>
    <template #footer><div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="!canSubmit" @click="submit">{{ submitting ? t('common.saving') : t('common.save') }}</button></div></template>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Group } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'
const props = defineProps<{ show: boolean; selectedIds: number[] }>()
const emit = defineEmits<{ close: []; success: [affected: number] }>()
const { t } = useI18n()
const appStore = useAppStore()
const groups = ref<Group[]>([])
const hiddenIds = ref<number[]>([])
const submitting = ref(false)
const tooLarge = computed(() => props.selectedIds.length > 500)
const canSubmit = computed(() => props.selectedIds.length > 0 && !tooLarge.value && !submitting.value)

watch(
  () => props.show,
  async (visible) => {
    if (!visible) return
    hiddenIds.value = []
    try {
      const response = await adminAPI.groups.list(1, 1000)
      groups.value = response.items.filter(
        (group) => group.status === 'active' && group.subscription_type === 'standard'
      )
    } catch (error) {
      console.error('Failed to load model groups:', error)
      appStore.showError(t('admin.users.failedToLoadGroups'))
    }
  }
)

const submit = async () => {
  if (!canSubmit.value) return
  submitting.value = true
  try {
    const response = await adminAPI.users.batchUpdateHiddenGroups({
      user_ids: props.selectedIds,
      group_ids: hiddenIds.value
    })
    appStore.showSuccess(t('admin.users.bulkHiddenGroups.success', { count: response.affected }))
    emit('success', response.affected)
    emit('close')
  } catch (error) {
    console.error('Failed to update hidden model groups:', error)
    appStore.showError(t('admin.users.bulkHiddenGroups.failed'))
  } finally {
    submitting.value = false
  }
}
</script>