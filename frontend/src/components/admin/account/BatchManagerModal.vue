<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="fixed inset-0 bg-black/50" @click="$emit('close')"></div>
        <div class="relative w-full max-w-2xl rounded-xl bg-white shadow-2xl dark:bg-gray-800 max-h-[80vh] flex flex-col">
          <!-- Header -->
          <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.batches.title', '批次管理') }}</h3>
            <button @click="$emit('close')" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Create form -->
          <div class="border-b border-gray-200 px-6 py-4 dark:border-gray-700">
            <div class="flex gap-3">
              <input
                v-model="newName"
                :placeholder="t('admin.batches.namePlaceholder', '批次名称')"
                class="flex-1 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                @keyup.enter="handleCreate"
              />
              <input
                v-model="newDescription"
                :placeholder="t('admin.batches.descPlaceholder', '描述（可选）')"
                class="flex-1 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                @keyup.enter="handleCreate"
              />
              <button
                @click="handleCreate"
                :disabled="!newName.trim() || creating"
                class="btn btn-primary px-4 py-2 text-sm"
              >
                {{ creating ? t('common.creating', '创建中...') : t('common.create', '创建') }}
              </button>
            </div>
          </div>

          <!-- List -->
          <div class="flex-1 overflow-y-auto px-6 py-2">
            <div v-if="loading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading', '加载中...') }}</div>
            <div v-else-if="batches.length === 0" class="py-8 text-center text-sm text-gray-500">
              {{ t('admin.batches.empty', '暂无批次，请在上方创建') }}
            </div>
            <div v-else class="divide-y divide-gray-100 dark:divide-gray-700">
              <div v-for="batch in batches" :key="batch.id" class="flex items-center gap-3 py-3">
                <div v-if="editingId === batch.id" class="flex flex-1 gap-2">
                  <input
                    v-model="editName"
                    class="w-1/3 rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                    @keyup.enter="handleUpdate(batch.id)"
                  />
                  <input
                    v-model="editDescription"
                    class="flex-1 rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                    :placeholder="t('admin.batches.descPlaceholder', '描述')"
                    @keyup.enter="handleUpdate(batch.id)"
                  />
                  <button @click="handleUpdate(batch.id)" class="btn btn-primary px-3 py-1.5 text-xs">{{ t('common.save', '保存') }}</button>
                  <button @click="editingId = null" class="btn btn-secondary px-3 py-1.5 text-xs">{{ t('common.cancel', '取消') }}</button>
                </div>
                <template v-else>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="font-medium text-gray-900 dark:text-white text-sm">{{ batch.name }}</span>
                      <span class="rounded-full bg-teal-50 px-2 py-0.5 text-xs text-teal-700 dark:bg-teal-900/30 dark:text-teal-300">
                        {{ batch.account_count }} {{ t('admin.batches.accounts', '个账号') }}
                      </span>
                      <span class="text-xs text-gray-400">{{ batch.source }}</span>
                    </div>
                    <div v-if="batch.description" class="text-xs text-gray-500 dark:text-gray-400 truncate">{{ batch.description }}</div>
                  </div>
                  <div class="flex items-center gap-1">
                    <button @click="startEdit(batch)" class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300" :title="t('common.edit', '编辑')">
                      <Icon name="edit" size="sm" />
                    </button>
                    <button @click="handleDelete(batch)" class="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" :title="t('common.delete', '删除')">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import * as batchesAPI from '@/api/admin/batches'
import type { Batch } from '@/api/admin/batches'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits(['close'])
const { t } = useI18n()

const batches = ref<Batch[]>([])
const loading = ref(false)
const creating = ref(false)
const newName = ref('')
const newDescription = ref('')
const editingId = ref<number | null>(null)
const editName = ref('')
const editDescription = ref('')

const loadBatches = async () => {
  loading.value = true
  try {
    const res = await batchesAPI.list()
    batches.value = res ?? []
  } finally {
    loading.value = false
  }
}

watch(() => props.show, (v) => {
  if (v) loadBatches()
})

const handleCreate = async () => {
  if (!newName.value.trim()) return

  // 前端唯一性检查
  if (batches.value.some(b => b.name === newName.value.trim())) {
    alert(t('admin.batches.nameExists', '该批次名称已存在，请使用其他名称'))
    return
  }

  creating.value = true
  try {
    await batchesAPI.create({ name: newName.value.trim(), description: newDescription.value.trim() })
    newName.value = ''
    newDescription.value = ''
    await loadBatches()
  } catch (error: any) {
    // 后端唯一性检查
    const msg = error?.message || ''
    if (msg.includes('already exists') || msg.includes('duplicate') || msg.includes('unique')) {
      alert(t('admin.batches.nameExists', '该批次名称已存在，请使用其他名称'))
    }
  } finally {
    creating.value = false
  }
}

const startEdit = (batch: Batch) => {
  editingId.value = batch.id
  editName.value = batch.name
  editDescription.value = batch.description
}

const handleUpdate = async (id: number) => {
  if (!editName.value.trim()) return

  // 检查名称唯一性（排除自身）
  if (batches.value.some(b => b.id !== id && b.name === editName.value.trim())) {
    alert(t('admin.batches.nameExists', '该批次名称已存在，请使用其他名称'))
    return
  }

  try {
    await batchesAPI.update(id, { name: editName.value.trim(), description: editDescription.value.trim() })
    editingId.value = null
    await loadBatches()
  } catch (error: any) {
    const msg = error?.message || ''
    if (msg.includes('already exists') || msg.includes('duplicate') || msg.includes('unique')) {
      alert(t('admin.batches.nameExists', '该批次名称已存在，请使用其他名称'))
    }
  }
}

const handleDelete = async (batch: Batch) => {
  if (!confirm(t('admin.batches.deleteConfirm', `确定删除批次「${batch.name}」？批次下的账号不会被删除，但会解除批次关联。`))) return
  await batchesAPI.remove(batch.id)
  await loadBatches()
}
</script>
