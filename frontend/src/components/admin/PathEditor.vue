<template>
  <div class="space-y-3">
    <!-- 数组空时占位。errors.path='paths' 切红色 -->
    <div
      v-if="modelValue.length === 0"
      :class="[
        'rounded-lg border border-dashed px-4 py-3 text-center text-sm',
        errorFor('paths')
          ? 'border-red-400 bg-red-50 text-red-600 dark:border-red-500 dark:bg-red-900/10 dark:text-red-400'
          : 'border-gray-300 text-gray-500 dark:border-dark-600 dark:text-gray-400',
      ]"
    >
      <span v-if="errorFor('paths')">
        {{ t('admin.serviceQuota.errors.' + errorFor('paths')) }} · {{ t('admin.serviceQuota.pathEditor.empty') }}
      </span>
      <span v-else>{{ t('admin.serviceQuota.pathEditor.empty') }}</span>
    </div>
    <div
      v-for="(item, index) in modelValue"
      :key="item.uid ?? index"
      class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
    >
      <div class="mb-3 flex items-center justify-between">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.serviceQuota.pathEditor.pathIndex', { index: index + 1 }) }}
        </span>
        <button
          type="button"
          class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
          :title="t('common.delete')"
          @click="remove(index)"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>
      <div class="space-y-3">
        <div>
          <RequiredLabel :label="t('admin.serviceQuota.form.platform')" required class="mb-2 block" />
          <PlatformPicker
            :model-value="item.platform"
            @update:model-value="updateField(index, 'platform', $event)"
          />
          <span
            v-if="errorFor(`paths[${index}].platform`)"
            class="mt-1 block text-xs text-red-500"
          >
            {{ t('admin.serviceQuota.errors.' + errorFor(`paths[${index}].platform`)) }}
          </span>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.form.channelId') }}</span>
            <EntitySearchSelect
              :model-value="item.channel_id"
              :placeholder="t('common.optional')"
              :search="(kw, signal) => searchChannels(kw, signal, item.platform)"
              :resolve-label="resolveChannelLabel"
              :reset-token="item.platform ?? ''"
              @update:model-value="updateField(index, 'channel_id', $event)"
            />
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.form.groupId') }}</span>
            <EntitySearchSelect
              :model-value="item.group_id"
              :placeholder="t('common.optional')"
              :search="(kw, signal) => searchGroups(kw, signal, item.platform)"
              :resolve-label="resolveGroupLabel"
              :reset-token="`${item.platform ?? ''}:${item.channel_id ?? ''}`"
              @update:model-value="updateField(index, 'group_id', $event)"
            />
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.form.accountId') }}</span>
            <EntitySearchSelect
              :model-value="item.account_id"
              :placeholder="t('common.optional')"
              :search="(kw, signal) => searchAccounts(kw, signal, item.platform, item.group_id)"
              :resolve-label="resolveAccountLabel"
              :reset-token="`${item.platform ?? ''}:${item.channel_id ?? ''}:${item.group_id ?? ''}`"
              @update:model-value="updateField(index, 'account_id', $event)"
            />
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.form.modelPattern') }}</span>
            <input
              :value="item.model_pattern || ''"
              :placeholder="t('admin.serviceQuota.form.modelPatternPlaceholder')"
              :list="`model-options-${item.uid ?? index}`"
              class="input"
              @input="updateField(index, 'model_pattern', ($event.target as HTMLInputElement).value || null)"
            />
            <datalist :id="`model-options-${item.uid ?? index}`">
              <option
                v-for="m in modelOptionsFor(item)"
                :key="m"
                :value="m"
              />
            </datalist>
            <span class="text-[11px] text-gray-500 dark:text-gray-400">
              {{ t('admin.serviceQuota.form.modelPatternHint') }}
            </span>
          </label>
        </div>
      </div>
    </div>
    <button type="button" class="btn btn-secondary" @click="add">
      <Icon name="plus" size="sm" class="mr-2" />
      {{ t('admin.serviceQuota.pathEditor.add') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import EntitySearchSelect, { type EntitySearchItem } from '@/components/common/EntitySearchSelect.vue'
import PlatformPicker from '@/components/common/PlatformPicker.vue'
import RequiredLabel from '@/components/common/RequiredLabel.vue'
import adminAPI from '@/api/admin'
import type { Channel } from '@/api/admin/channels'
import type { GroupPlatform } from '@/types'
import type { ServiceQuotaPathInput } from '@/api/admin/serviceQuota'
import type { ValidationError } from '@/utils/validateServiceQuota'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    modelValue: ServiceQuotaPathInput[]
    /** 父组件传入的校验错误（路径与 validateServiceQuotaRule 输出对齐） */
    errors?: ValidationError[]
  }>(),
  { errors: () => [] },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: ServiceQuotaPathInput[]): void
}>()

function errorFor(path: string): string {
  return props.errors.find((e) => e.path === path)?.code || ''
}

// channel cache：搜索 / resolveLabel 都会写入；
// modelOptionsFor 只在 cache 命中时返回模型 datalist，避免组件内额外网络请求。
// 用 reactive 保证 cache 变更后 datalist 自动重渲染。
const channelCache = reactive(new Map<number, Channel>()) as Map<number, Channel>

// admin channels list 不支持 platform 过滤，这里走客户端过滤；
// page_size=200 已能覆盖绝大多数生产环境（>200 渠道很少见，超出时
// 提示用户用关键字精确搜索即可）。
const CHANNEL_FETCH_PAGE_SIZE = 200

function blankPath(): ServiceQuotaPathInput {
  return {
    uid: crypto.randomUUID(),
    platform: null,
    channel_id: null,
    group_id: null,
    account_id: null,
    model_pattern: null,
  }
}

function add() {
  emit('update:modelValue', [...props.modelValue, blankPath()])
}

function remove(index: number) {
  const next = props.modelValue.slice()
  next.splice(index, 1)
  emit('update:modelValue', next)
}

function updateField<K extends keyof ServiceQuotaPathInput>(index: number, key: K, value: ServiceQuotaPathInput[K]) {
  const next = props.modelValue.slice()
  const updated: ServiceQuotaPathInput = { ...next[index], [key]: value }
  // 改平台时清空下游字段（渠道/分组/账号/模型）
  if (key === 'platform') {
    updated.channel_id = null
    updated.group_id = null
    updated.account_id = null
    updated.model_pattern = null
  } else if (key === 'channel_id') {
    // 切换渠道时同样清掉模型 pattern，避免 datalist 候选与上一个 channel 错位
    updated.group_id = null
    updated.account_id = null
    updated.model_pattern = null
  } else if (key === 'group_id') {
    updated.account_id = null
  }
  next[index] = updated
  emit('update:modelValue', next)
}

function channelMatchesPlatform(channel: Channel, platform: string | null | undefined): boolean {
  if (!platform) return true
  return channel.model_pricing.some((mp) => mp.platform === platform)
}

async function searchChannels(
  keyword: string,
  signal: AbortSignal,
  platform: string | null | undefined,
): Promise<EntitySearchItem[]> {
  const filters: { search?: string } = {}
  if (keyword) filters.search = keyword
  const res = await adminAPI.channels.list(1, CHANNEL_FETCH_PAGE_SIZE, filters, { signal })
  for (const ch of res.items) channelCache.set(ch.id, ch)
  return res.items
    .filter((ch) => channelMatchesPlatform(ch, platform))
    .map((ch) => ({ id: ch.id, label: ch.name, sub: ch.status || '' }))
}

async function resolveChannelLabel(id: number): Promise<EntitySearchItem | null> {
  try {
    const res = await adminAPI.channels.getById(id)
    channelCache.set(res.id, res)
    return { id: res.id, label: res.name }
  } catch {
    return null
  }
}

// 仅当 cache 已有该 channel 时返回模型 datalist 候选；
// platform 不限定时合并所有 platform 下的 models（去重）。
function modelOptionsFor(item: ServiceQuotaPathInput): string[] {
  if (!item.channel_id) return []
  const channel = channelCache.get(item.channel_id)
  if (!channel) return []
  const target = item.platform || ''
  const set = new Set<string>()
  for (const mp of channel.model_pricing) {
    if (target && mp.platform !== target) continue
    for (const m of mp.models || []) {
      if (m) set.add(m)
    }
  }
  return Array.from(set).sort()
}

async function searchGroups(keyword: string, signal: AbortSignal, platform: string | null | undefined): Promise<EntitySearchItem[]> {
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

async function searchAccounts(keyword: string, signal: AbortSignal, platform: string | null | undefined, groupId: number | null | undefined): Promise<EntitySearchItem[]> {
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