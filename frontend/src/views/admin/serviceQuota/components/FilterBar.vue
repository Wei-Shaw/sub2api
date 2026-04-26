<template>
  <div class="flex flex-wrap items-end gap-3">
    <!-- Rule -->
    <label class="form-field min-w-[180px]">
      <span class="input-label">{{ t('admin.serviceQuotaMonitor.filters.rule') }}</span>
      <select
        :value="modelValue.rule_id ?? ''"
        class="input"
        @change="onSelectChange('rule_id', $event)"
      >
        <option value="">{{ t('common.all') }}</option>
        <option v-for="rule in rules" :key="rule.id" :value="rule.id">
          {{ rule.name || `#${rule.id}` }}
        </option>
      </select>
    </label>

    <!-- Platform -->
    <label class="form-field min-w-[200px]">
      <span class="input-label">{{ t('admin.serviceQuotaMonitor.filters.platform') }}</span>
      <PlatformPicker
        :model-value="modelValue.platform ?? null"
        @update:model-value="updateField('platform', $event)"
      />
    </label>

    <!-- User -->
    <label class="form-field w-[200px]">
      <span class="input-label">{{ t('admin.serviceQuotaMonitor.filters.user') }}</span>
      <EntitySearchSelect
        :model-value="modelValue.user_id ?? null"
        :placeholder="t('common.optional')"
        :search="searchUsers"
        :resolve-label="resolveUserLabel"
        @update:model-value="updateField('user_id', $event)"
      />
    </label>

    <!-- Channel -->
    <label class="form-field w-[200px]">
      <span class="input-label">{{ t('admin.serviceQuotaMonitor.filters.channel') }}</span>
      <EntitySearchSelect
        :model-value="modelValue.channel_id ?? null"
        :placeholder="t('common.optional')"
        :search="searchChannels"
        :resolve-label="resolveChannelLabel"
        @update:model-value="updateField('channel_id', $event)"
      />
    </label>

    <!-- Group -->
    <label class="form-field w-[200px]">
      <span class="input-label">{{ t('admin.serviceQuotaMonitor.filters.group') }}</span>
      <EntitySearchSelect
        :model-value="modelValue.group_id ?? null"
        :placeholder="t('common.optional')"
        :search="(kw, signal) => searchGroups(kw, signal, modelValue.platform ?? null)"
        :resolve-label="resolveGroupLabel"
        :reset-token="modelValue.platform ?? ''"
        @update:model-value="updateField('group_id', $event)"
      />
    </label>

    <!-- Account -->
    <label class="form-field w-[200px]">
      <span class="input-label">{{ t('admin.serviceQuotaMonitor.filters.account') }}</span>
      <EntitySearchSelect
        :model-value="modelValue.account_id ?? null"
        :placeholder="t('common.optional')"
        :search="(kw, signal) => searchAccounts(kw, signal, modelValue.platform ?? null, modelValue.group_id ?? null)"
        :resolve-label="resolveAccountLabel"
        :reset-token="`${modelValue.platform ?? ''}:${modelValue.group_id ?? ''}`"
        @update:model-value="updateField('account_id', $event)"
      />
    </label>

    <button
      type="button"
      class="btn btn-secondary"
      :disabled="!hasAnyFilter"
      @click="clearAll"
    >
      <Icon name="x" size="sm" class="mr-1" />
      {{ t('admin.serviceQuotaMonitor.filters.clear') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import EntitySearchSelect, { type EntitySearchItem } from '@/components/common/EntitySearchSelect.vue'
import PlatformPicker from '@/components/common/PlatformPicker.vue'
import adminAPI from '@/api/admin'
import type { GroupPlatform } from '@/types'
import {
  listServiceQuotaRules,
  type ServiceQuotaMonitorFilter,
  type ServiceQuotaRule,
} from '@/api/admin/serviceQuota'

const props = defineProps<{
  modelValue: ServiceQuotaMonitorFilter
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: ServiceQuotaMonitorFilter): void
}>()

const { t } = useI18n()
const rules = ref<ServiceQuotaRule[]>([])

onMounted(async () => {
  try {
    rules.value = await listServiceQuotaRules()
  } catch (err) {
    console.error('[ServiceQuotaMonitor.FilterBar] Failed to load rules', err)
    rules.value = []
  }
})

const hasAnyFilter = computed(() => {
  return Object.values(props.modelValue).some((v) => {
    if (v === undefined || v === null) return false
    if (typeof v === 'string' && v.trim() === '') return false
    return true
  })
})

function emitNext(next: ServiceQuotaMonitorFilter): void {
  emit('update:modelValue', next)
}

function updateField<K extends keyof ServiceQuotaMonitorFilter>(
  key: K,
  value: ServiceQuotaMonitorFilter[K] | null
): void {
  const next: ServiceQuotaMonitorFilter = { ...props.modelValue }
  if (value === null || value === undefined || value === '') {
    delete next[key]
  } else {
    next[key] = value
  }
  // 改 platform 时清空下游 group/account
  if (key === 'platform') {
    delete next.group_id
    delete next.account_id
  }
  if (key === 'group_id') {
    delete next.account_id
  }
  emitNext(next)
}

function onSelectChange(key: keyof ServiceQuotaMonitorFilter, ev: Event): void {
  const raw = (ev.target as HTMLSelectElement).value
  if (!raw) {
    updateField(key, null)
    return
  }
  if (key === 'platform') {
    updateField(key, raw)
    return
  }
  const n = Number.parseInt(raw, 10)
  updateField(key, Number.isFinite(n) ? n : null)
}

function clearAll(): void {
  emitNext({})
}

// ---- Entity searchers (与 PathEditor 保持一致风格) ----

async function searchUsers(keyword: string, signal: AbortSignal): Promise<EntitySearchItem[]> {
  const filters: { search?: string } = {}
  if (keyword) filters.search = keyword
  const res = await adminAPI.users.list(1, 20, filters, { signal })
  return res.items.map((u) => ({ id: u.id, label: u.email || u.username || `#${u.id}`, sub: u.username || '' }))
}

async function resolveUserLabel(id: number): Promise<EntitySearchItem | null> {
  try {
    const res = await adminAPI.users.getById(id)
    return { id: res.id, label: res.email || res.username || `#${res.id}` }
  } catch {
    return null
  }
}

async function searchChannels(keyword: string, signal: AbortSignal): Promise<EntitySearchItem[]> {
  const filters: { search?: string } = {}
  if (keyword) filters.search = keyword
  const res = await adminAPI.channels.list(1, 20, filters, { signal })
  return res.items.map((ch) => ({ id: ch.id, label: ch.name, sub: ch.status || '' }))
}

async function resolveChannelLabel(id: number): Promise<EntitySearchItem | null> {
  try {
    const res = await adminAPI.channels.getById(id)
    return { id: res.id, label: res.name }
  } catch {
    return null
  }
}

async function searchGroups(keyword: string, signal: AbortSignal, platform: string | null): Promise<EntitySearchItem[]> {
  const filters: { search?: string; platform?: GroupPlatform } = {}
  if (keyword) filters.search = keyword
  if (platform) filters.platform = platform as GroupPlatform
  const res = await adminAPI.groups.list(1, 20, filters, { signal })
  return res.items.map((g) => ({ id: g.id, label: g.name, sub: g.platform || '' }))
}

async function resolveGroupLabel(id: number): Promise<EntitySearchItem | null> {
  try {
    const res = await adminAPI.groups.getById(id)
    return { id: res.id, label: res.name, sub: res.platform || '' }
  } catch {
    return null
  }
}

async function searchAccounts(
  keyword: string,
  signal: AbortSignal,
  platform: string | null,
  groupId: number | null
): Promise<EntitySearchItem[]> {
  const filters: Record<string, string> = {}
  if (keyword) filters.search = keyword
  if (platform) filters.platform = platform
  if (groupId) filters.group = String(groupId)
  const res = await adminAPI.accounts.list(1, 20, filters, { signal })
  return res.items.map((a) => ({ id: a.id, label: a.name, sub: a.platform || '' }))
}

async function resolveAccountLabel(id: number): Promise<EntitySearchItem | null> {
  try {
    const res = await adminAPI.accounts.getById(id)
    return { id: res.id, label: res.name, sub: res.platform || '' }
  } catch {
    return null
  }
}
</script>

<style scoped>
.form-field {
  @apply space-y-1.5;
}
</style>
