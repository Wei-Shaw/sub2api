<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="px-6 py-4 flex-shrink-0">
      <div class="flex flex-wrap items-end gap-4">
        <div class="min-w-[180px]">
          <label class="input-label">{{ t('usage.errors.model') }}</label>
          <Select
            v-model="localModel"
            :options="modelOptions"
            searchable
            creatable
            clearable
            :placeholder="t('usage.errors.modelPlaceholder')"
            @change="apply"
          />
        </div>
        <div class="min-w-[160px]">
          <label class="input-label">{{ t('usage.errors.keyName') }}</label>
          <Select
            v-model="localApiKeyId"
            :options="keyOptions"
            :placeholder="t('usage.errors.allKeys')"
            @change="apply"
          />
        </div>
        <div class="min-w-[140px]">
          <label class="input-label">{{ t('usage.errors.category') }}</label>
          <Select
            v-model="localCategory"
            :options="categoryOptions"
            :placeholder="t('usage.errors.allCategories')"
            @change="apply"
          />
        </div>
        <button class="btn btn-primary" @click="apply">
          <Icon name="search" size="sm" />
          {{ t('common.search') }}
        </button>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-auto">
      <table class="min-w-full text-sm">
        <thead>
          <tr>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.model') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.keyName') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.endpoint') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.status') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.category') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.message') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.platform') }}</th>
            <th class="px-4 py-2 text-left">{{ t('usage.errors.time') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, i) in rows"
            :key="i"
            class="border-t border-gray-100 dark:border-dark-700 cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-800"
            @click="openDetail(row.id)"
          >
            <td class="px-4 py-2">{{ row.model || '-' }}</td>
            <td class="px-4 py-2">
              <span>{{ row.key_name || '-' }}</span>
              <span
                v-if="row.key_deleted"
                class="ml-1 inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              >{{ t('usage.errors.keyDeleted') }}</span>
            </td>
            <td class="px-4 py-2">{{ row.inbound_endpoint || '-' }}</td>
            <td class="px-4 py-2"><span class="badge" :class="statusClass(row.status_code)">{{ row.status_code || '-' }}</span></td>
            <td class="px-4 py-2">{{ t('usage.errors.categories.' + row.category) }}</td>
            <td class="px-4 py-2 max-w-[280px] truncate" :title="row.message">{{ row.message || '-' }}</td>
            <td class="px-4 py-2">{{ row.platform || '-' }}</td>
            <td class="px-4 py-2">{{ formatDateTime(row.created_at) }}</td>
          </tr>
          <tr v-if="!loading && rows.length === 0">
            <td colspan="8" class="px-4 py-8 text-center text-gray-400">{{ t('usage.errors.empty') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex-shrink-0">
      <Pagination :page="page" :page-size="pageSize" :total="total"
        @update:page="$emit('update:page', $event)"
        @update:pageSize="$emit('update:pageSize', $event)" />
    </div>

    <UserErrorDetailModal v-model:show="showDetail" :error-id="selectedId" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import UserErrorDetailModal from '@/components/user/UserErrorDetailModal.vue'
import { formatDateTime } from '@/utils/format'
import type { UserErrorRequest, ApiKey } from '@/types'

const props = defineProps<{
  rows: UserErrorRequest[]
  total: number
  loading: boolean
  page: number
  pageSize: number
  apiKeys?: ApiKey[]
}>()

const emit = defineEmits<{
  (e: 'update:page', v: number): void
  (e: 'update:pageSize', v: number): void
  (e: 'filter', v: { model: string; category: string; api_key_id: number | null }): void
}>()

const { t } = useI18n()
// string | null:clearable 清空时 Select 回传 null,apply 中归一为空串
const localModel = ref<string | null>('')
const localCategory = ref<string>('')
const localApiKeyId = ref<number | null>(null)

const categoryCodes = ['auth', 'rate_limit', 'quota', 'invalid_request', 'service_unavailable', 'upstream', 'internal']

const categoryOptions = computed(() => [
  { value: '', label: t('usage.errors.allCategories') },
  ...categoryCodes.map((c) => ({ value: c, label: t('usage.errors.categories.' + c) })),
])

// 首项 value: null 表示不按 key 过滤；其余项取自父组件传入的 apiKeys 候选列表。
const keyOptions = computed(() => [
  { value: null, label: t('usage.errors.allKeys') },
  ...(props.apiKeys ?? []).map((k) => ({ value: k.id, label: k.name })),
])

// 模型候选取自当前已加载错误中出现过的模型；creatable 允许输入任意片段做后端模糊。
const modelOptions = computed(() => {
  const seen = new Set<string>()
  const opts: { value: string; label: string }[] = []
  for (const r of props.rows) {
    if (r.model && !seen.has(r.model)) {
      seen.add(r.model)
      opts.push({ value: r.model, label: r.model })
    }
  }
  return opts
})

const showDetail = ref(false)
const selectedId = ref<number | null>(null)

function openDetail(id: number) {
  selectedId.value = id
  showDetail.value = true
}

function apply() {
  emit('filter', {
    model: (localModel.value ?? '').trim(),
    category: localCategory.value || '',
    api_key_id: localApiKeyId.value,
  })
}

function statusClass(code: number) {
  if (code >= 500) return 'badge-danger'
  if (code === 429) return 'badge-warning'
  return 'badge-gray'
}
</script>
