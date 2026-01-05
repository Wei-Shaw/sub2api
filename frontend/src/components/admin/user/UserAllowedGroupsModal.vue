<template>
  <BaseDialog :show="show" :title="t('admin.users.setAllowedGroups')" width="normal" @close="$emit('close')">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100">
          <span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() REDACTEDREDACTED</span>
        </div>
        <p class="font-medium text-gray-900 dark:text-white">{{ user.email REDACTEDREDACTED</p>
      </div>
      <div v-if="loading" class="flex justify-center py-8"><svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg></div>
      <div v-else>
        <p class="mb-3 text-sm text-gray-600">{{ t('admin.users.allowedGroupsHint') REDACTEDREDACTED</p>
        <div class="max-h-64 space-y-2 overflow-y-auto">
          <label v-for="group in groups" :key="group.id" class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 p-3 hover:bg-gray-50" :class="{'border-primary-300 bg-primary-50': selectedIds.includes(group.id)REDACTED">
            <input type="checkbox" :value="group.id" v-model="selectedIds" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
            <div class="flex-1"><p class="font-medium text-gray-900">{{ group.name REDACTEDREDACTED</p><p v-if="group.description" class="truncate text-sm text-gray-500">{{ group.description REDACTEDREDACTED</p></div>
            <div class="flex items-center gap-2"><span class="badge badge-gray text-xs">{{ group.platform REDACTEDREDACTED</span><span v-if="group.is_exclusive" class="badge badge-purple text-xs">{{ t('admin.groups.exclusive') REDACTEDREDACTED</span></div>
          </label>
        </div>
        <div class="mt-4 border-t border-gray-200 pt-4">
          <label class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 p-3 hover:bg-gray-50" :class="{'border-green-300 bg-green-50': selectedIds.length === 0REDACTED">
            <input type="radio" :checked="selectedIds.length === 0" @change="selectedIds = []" class="h-4 w-4 border-gray-300 text-green-600" />
            <div class="flex-1"><p class="font-medium text-gray-900">{{ t('admin.users.allowAllGroups') REDACTEDREDACTED</p><p class="text-sm text-gray-500">{{ t('admin.users.allowAllGroupsHint') REDACTEDREDACTED</p></div>
          </label>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" class="btn btn-secondary">{{ t('common.cancel') REDACTEDREDACTED</button>
        <button @click="handleSave" :disabled="submitting" class="btn btn-primary">{{ submitting ? t('common.saving') : t('common.save') REDACTEDREDACTED</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { User, Group REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean, user: User | null REDACTED>()
const emit = defineEmits(['close', 'success']); const { t REDACTED = useI18n(); const appStore = useAppStore()

const groups = ref<Group[]>([]); const selectedIds = ref<number[]>([]); const loading = ref(false); const submitting = ref(false)

watch(() => props.show, (v) => { if(v && props.user) { selectedIds.value = props.user.allowed_groups || []; load() REDACTED REDACTED)
const load = async () => { loading.value = true; try { const res = await adminAPI.groups.list(1, 1000); groups.value = res.items.filter(g => g.subscription_type === 'standard' && g.status === 'active') REDACTED catch {REDACTED finally { loading.value = false REDACTED REDACTED
const handleSave = async () => {
  if (!props.user) return; submitting.value = true
  try {
    await adminAPI.users.update(props.user.id, { allowed_groups: selectedIds.value REDACTED)
    appStore.showSuccess(t('admin.users.allowedGroupsUpdated')); emit('success'); emit('close')
  REDACTED catch {REDACTED finally { submitting.value = false REDACTED
REDACTED
</script>