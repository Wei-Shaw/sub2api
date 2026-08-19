<template>
  <BaseDialog :show="show" :title="t('admin.users.bulkTags.title')" width="normal" @close="emit('close')">
    <form id="bulk-tags-form" class="space-y-4" @submit.prevent="submit">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.users.bulkTags.selectedCount', { count: selectedIds.length }) }}</p>
      <Select v-model="mode" :options="modeOptions" />
      <div class="flex justify-end"><button type="button" class="btn btn-secondary" @click="createNew">{{ t('admin.users.bulkTags.newTag') }}</button></div>
      <div class="flex flex-wrap gap-2"><label v-for="tag in tags" :key="tag.id" class="flex items-center gap-2 rounded-lg border px-3 py-2"><input v-model="tagIds" type="checkbox" :value="tag.id" /><span class="badge" :style="{ backgroundColor: tag.color, color: '#fff' }">{{ tag.name }}</span></label></div>
      <p v-if="tooLarge" class="text-sm text-red-600">{{ t('admin.users.bulkTags.selectionLimit', { max: 500 }) }}</p>
    </form>
    <template #footer><div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button><button form="bulk-tags-form" class="btn btn-primary" :disabled="!canSubmit">{{ submitting ? t('common.saving') : t('common.save') }}</button></div></template>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { UserTag } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores/app'
const props = defineProps<{ show: boolean; selectedIds: number[] }>()
const emit = defineEmits<{ close: []; success: [affected: number] }>()
const { t } = useI18n(); const appStore = useAppStore(); const tags = ref<UserTag[]>([]); const tagIds = ref<number[]>([]); const mode = ref<'add'|'remove'|'replace'>('add'); const submitting = ref(false)
const modeOptions = computed(() => [{ value: 'add', label: t('admin.users.bulkTags.add') }, { value: 'remove', label: t('admin.users.bulkTags.remove') }, { value: 'replace', label: t('admin.users.bulkTags.replace') }])
const tooLarge = computed(() => props.selectedIds.length > 500); const canSubmit = computed(() => props.selectedIds.length > 0 && !tooLarge.value && !submitting.value && (mode.value === 'replace' || tagIds.value.length > 0))
watch(() => props.show, async v => {
  if (!v) return
  tagIds.value = []
  mode.value = 'add'
  try {
    tags.value = await adminAPI.users.listTags()
  } catch (error) {
    console.error('Failed to load user tags:', error)
    appStore.showError(t('admin.users.bulkTags.failed'))
  }
})
const createNew = async () => { const name = window.prompt(t('admin.users.bulkTags.newTagName')); if (!name?.trim()) return; try { const tag = await adminAPI.users.createTag({ name: name.trim() }); tags.value.push(tag); tagIds.value.push(tag.id) } catch { appStore.showError(t('admin.users.bulkTags.failed')) } }
const submit = async () => { if (!canSubmit.value) return; submitting.value = true; try { const r = await adminAPI.users.batchUpdateTags({ user_ids: props.selectedIds, tag_ids: tagIds.value, mode: mode.value }); appStore.showSuccess(t('admin.users.bulkTags.success', { count: r.affected })); emit('success', r.affected); emit('close') } catch { appStore.showError(t('admin.users.bulkTags.failed')) } finally { submitting.value = false } }
</script>