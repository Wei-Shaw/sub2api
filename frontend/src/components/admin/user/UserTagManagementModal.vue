<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.tagManagement.title')"
    width="wide"
    @close="emit('close')"
  >
    <form class="grid gap-4 border-b border-gray-200 pb-5 dark:border-dark-600 sm:grid-cols-[minmax(0,1fr)_8rem_minmax(0,1.4fr)_auto]" @submit.prevent="saveTag">
      <div>
        <label class="input-label" for="user-tag-name">{{ t('admin.users.tagManagement.name') }}</label>
        <input
          id="user-tag-name"
          v-model="form.name"
          class="input"
          maxlength="80"
          required
        />
      </div>
      <div>
        <label class="input-label" for="user-tag-color">{{ t('admin.users.tagManagement.color') }}</label>
        <input
          id="user-tag-color"
          v-model="form.color"
          type="color"
          class="h-11 w-full cursor-pointer rounded-md border border-gray-200 bg-white p-1 dark:border-dark-600 dark:bg-dark-800"
        />
      </div>
      <div>
        <label class="input-label" for="user-tag-description">{{ t('admin.users.tagManagement.description') }}</label>
        <input
          id="user-tag-description"
          v-model="form.description"
          class="input"
          maxlength="500"
        />
      </div>
      <div class="flex items-end gap-2">
        <button type="submit" class="btn btn-primary h-11" :disabled="saving || !form.name.trim()">
          {{ editingTag ? t('common.save') : t('common.create') }}
        </button>
        <button
          v-if="editingTag"
          type="button"
          class="btn btn-secondary h-11 px-3"
          :title="t('common.cancel')"
          @click="resetForm"
        >
          <Icon name="x" size="sm" />
        </button>
      </div>
    </form>

    <div class="mt-5 min-h-32">
      <div v-if="loading" class="flex justify-center py-8 text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="tags.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.users.tagManagement.empty') }}
      </div>
      <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
        <div v-for="tag in tags" :key="tag.id" class="flex min-h-16 items-center gap-3 py-3">
          <span class="h-5 w-5 flex-none rounded border border-black/10" :style="{ backgroundColor: tag.color }"></span>
          <div class="min-w-0 flex-1">
            <div class="truncate text-sm font-medium text-gray-900 dark:text-gray-100">{{ tag.name }}</div>
            <div v-if="tag.description" class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">{{ tag.description }}</div>
          </div>
          <button
            type="button"
            class="rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100"
            :title="t('common.edit')"
            @click="editTag(tag)"
          >
            <Icon name="edit" size="sm" />
          </button>
          <button
            type="button"
            class="rounded-md p-2 text-red-500 hover:bg-red-50 hover:text-red-700 dark:hover:bg-red-950/30"
            :title="t('common.delete')"
            @click="deletingTag = tag"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="Boolean(deletingTag)"
    :title="t('admin.users.tagManagement.deleteTitle')"
    :message="t('admin.users.tagManagement.deleteConfirm', { name: deletingTag?.name || '' })"
    :confirm-text="t('common.delete')"
    danger
    @confirm="deleteSelectedTag"
    @cancel="deletingTag = null"
  />
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { UserTag } from '@/types'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t } = useI18n()
const appStore = useAppStore()

const tags = ref<UserTag[]>([])
const loading = ref(false)
const saving = ref(false)
const editingTag = ref<UserTag | null>(null)
const deletingTag = ref<UserTag | null>(null)
const form = reactive({ name: '', color: '#6366f1', description: '' })

const resetForm = () => {
  editingTag.value = null
  form.name = ''
  form.color = '#6366f1'
  form.description = ''
}

const loadTags = async () => {
  loading.value = true
  try {
    tags.value = await adminAPI.users.listTags()
  } catch {
    appStore.showError(t('admin.users.tagManagement.loadFailed'))
  } finally {
    loading.value = false
  }
}

const editTag = (tag: UserTag) => {
  editingTag.value = tag
  form.name = tag.name
  form.color = tag.color
  form.description = tag.description || ''
}

const saveTag = async () => {
  const name = form.name.trim()
  if (!name || saving.value) return
  saving.value = true
  try {
    const input = { name, color: form.color, description: form.description.trim() }
    if (editingTag.value) {
      await adminAPI.users.updateTag(editingTag.value.id, input)
      appStore.showSuccess(t('admin.users.tagManagement.updated'))
    } else {
      await adminAPI.users.createTag(input)
      appStore.showSuccess(t('admin.users.tagManagement.created'))
    }
    resetForm()
    await loadTags()
    emit('changed')
  } catch {
    appStore.showError(t('admin.users.tagManagement.saveFailed'))
  } finally {
    saving.value = false
  }
}

const deleteSelectedTag = async () => {
  if (!deletingTag.value) return
  const tagID = deletingTag.value.id
  try {
    await adminAPI.users.deleteTag(tagID)
    if (editingTag.value?.id === tagID) resetForm()
    deletingTag.value = null
    await loadTags()
    emit('changed')
    appStore.showSuccess(t('admin.users.tagManagement.deleted'))
  } catch {
    appStore.showError(t('admin.users.tagManagement.deleteFailed'))
  }
}

watch(
  () => props.show,
  (show) => {
    if (!show) return
    resetForm()
    deletingTag.value = null
    void loadTags()
  }
)
</script>
