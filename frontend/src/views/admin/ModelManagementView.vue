<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold">{{ text('模型批量管理', 'Bulk model management') }}</h1>
          <p class="mt-2 text-sm text-gray-500">{{ text('批量维护渠道模型可用性和路由规则。变更直接写入所选渠道。', 'Bulk-maintain model availability and routing rules for selected channels.') }}</p>
        </div>
        <button class="btn btn-secondary" :disabled="loading || applying" @click="loadChannels">
          {{ text('刷新', 'Refresh') }}
        </button>
      </div>

      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-green-50 p-3 text-green-700 dark:bg-green-950">{{ notice }}</p>

      <section class="card space-y-5 p-5">
        <div class="flex flex-wrap gap-2" role="tablist" :aria-label="text('模型操作', 'Model operation')">
          <button v-for="item in operationOptions" :key="item.value" type="button" class="btn" :class="operation === item.value ? 'btn-primary' : 'btn-secondary'" @click="operation = item.value">
            {{ item.label }}
          </button>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <label class="block text-sm">
            {{ text('平台', 'Platform') }}
            <select v-model="platform" class="input mt-1">
              <option v-for="item in platformOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
          </label>
          <label class="block text-sm">
            {{ operation === 'route' ? text('源模型', 'Source model') : text('模型', 'Model') }}
            <input v-model.trim="sourceModel" list="model-candidates" class="input mt-1" :placeholder="text('例如 gpt-6-astra', 'e.g. gpt-6-astra')" />
          </label>
          <label v-if="operation === 'route'" class="block text-sm">
            {{ text('目标模型', 'Target model') }}
            <input v-model.trim="targetModel" class="input mt-1" :placeholder="text('例如 gpt-5.6-sol', 'e.g. gpt-5.6-sol')" />
          </label>
        </div>
        <datalist id="model-candidates">
          <option v-for="model in modelCandidates" :key="model" :value="model" />
        </datalist>

        <label v-if="operation === 'remove'" class="flex items-start gap-2 text-sm">
          <input v-model="enforceRestriction" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600" />
          <span>
            {{ text('下架后启用模型限制，确保该模型不再被渠道接受', 'Enable model restriction after removal so the model is no longer accepted') }}
            <span class="block text-xs text-gray-500">{{ text('关闭后仅移除定价项，未启用限制的渠道仍可能透传该模型。', 'When disabled, channels without model restriction may still pass the model through.') }}</span>
          </span>
        </label>

        <div class="border-t pt-4 dark:border-dark-600">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 class="font-medium">{{ text('应用到渠道', 'Apply to channels') }}</h2>
              <p class="text-xs text-gray-500">{{ selectedCount }} / {{ channels.length }} {{ text('个渠道已选择', 'channels selected') }}</p>
            </div>
            <div class="flex gap-2">
              <button type="button" class="btn btn-secondary text-xs" @click="selectAll">{{ text('全选', 'Select all') }}</button>
              <button type="button" class="btn btn-secondary text-xs" @click="clearSelection">{{ text('清空', 'Clear') }}</button>
            </div>
          </div>
          <div v-if="loading" class="py-5 text-center text-sm text-gray-500">{{ text('加载中…', 'Loading…') }}</div>
          <div v-else-if="!channels.length" class="py-5 text-sm text-gray-500">{{ text('暂无渠道', 'No channels') }}</div>
          <div v-else class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            <label v-for="channel in channels" :key="channel.id" class="flex cursor-pointer items-start gap-2 rounded border p-3 text-sm dark:border-dark-600" :class="selectedChannelIds.has(channel.id) ? 'border-primary-400 bg-primary-50 dark:bg-primary-950/30' : ''">
              <input type="checkbox" :checked="selectedChannelIds.has(channel.id)" class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600" @change="toggleChannel(channel.id)" />
              <span class="min-w-0"><span class="block truncate font-medium">{{ channel.name }}</span><span class="text-xs text-gray-500">{{ channel.status }} · {{ channel.model_pricing.filter(p => p.platform === platform).length }} {{ text('条定价', 'pricing rules') }}</span></span>
            </label>
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-3 border-t pt-4 dark:border-dark-600">
          <p class="text-sm text-gray-500">{{ previewText }}</p>
          <button class="btn btn-primary" :disabled="applying || loading || !canApply" @click="applyOperation">
            {{ applying ? text('处理中…', 'Applying…') : text('执行批量变更', 'Apply bulk change') }}
          </button>
        </div>
      </section>

      <section class="card p-5">
        <h2 class="mb-3 font-medium">{{ text('当前模型与路由', 'Current models and routes') }}</h2>
        <div v-if="modelCandidates.length" class="flex flex-wrap gap-2">
          <button v-for="model in modelCandidates" :key="model" type="button" class="rounded border px-2 py-1 text-xs hover:border-primary-400 dark:border-dark-600" @click="sourceModel = model">{{ model }}</button>
        </div>
        <p v-else class="text-sm text-gray-500">{{ text('所选平台暂无模型定价。', 'No priced models for the selected platform.') }}</p>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type { Channel, ChannelModelPricing } from '@/api/admin/channels'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'

type Operation = 'add' | 'remove' | 'route'
const { locale } = useI18n()
const appStore = useAppStore()
const text = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const operation = ref<Operation>('add')
const platform = ref('openai')
const sourceModel = ref('')
const targetModel = ref('')
const enforceRestriction = ref(true)
const channels = ref<Channel[]>([])
const selectedChannelIds = ref(new Set<number>())
const loading = ref(false)
const applying = ref(false)
const error = ref('')
const notice = ref('')

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'grok', label: 'Grok' },
  { value: 'kimi', label: 'Kimi' },
  { value: 'zhipu', label: 'Zhipu' },
  { value: 'deepseek', label: 'DeepSeek' },
  { value: 'minimax', label: 'MiniMax' },
  { value: 'opencode_go', label: 'OpenCode Go' }
]
const operationOptions = computed(() => [
  { value: 'add' as const, label: text('添加模型', 'Add model') },
  { value: 'remove' as const, label: text('下架模型', 'Remove model') },
  { value: 'route' as const, label: text('路由模型', 'Route model') }
])
const selectedCount = computed(() => selectedChannelIds.value.size)
const modelCandidates = computed(() => {
  const result = new Set<string>()
  for (const channel of channels.value) {
    for (const pricing of channel.model_pricing) {
      if (pricing.platform === platform.value) pricing.models.forEach(model => result.add(model))
    }
  }
  return [...result].sort((a, b) => a.localeCompare(b))
})
const affectedChannels = computed(() => channels.value.filter(channel => selectedChannelIds.value.has(channel.id)))
const canApply = computed(() => {
  if (!selectedCount.value || !sourceModel.value) return false
  return operation.value !== 'route' || (!!targetModel.value && !sameModel(sourceModel.value, targetModel.value))
})
const previewText = computed(() => {
  if (!canApply.value) return text('请选择渠道并填写模型。', 'Select channels and enter a model.')
  const count = affectedChannels.value.filter(channel => operationWouldChange(channel)).length
  return `${count} ${text('个渠道将被更新', 'channel(s) will be updated')}`
})

function selectAll() { selectedChannelIds.value = new Set(channels.value.map(channel => channel.id)) }
function clearSelection() { selectedChannelIds.value = new Set() }
function toggleChannel(id: number) {
  const next = new Set(selectedChannelIds.value)
  if (next.has(id)) next.delete(id); else next.add(id)
  selectedChannelIds.value = next
}
function sameModel(left: string, right: string) {
  return left.trim().toLowerCase() === right.trim().toLowerCase()
}
function findMappingKey(mapping: Record<string, string>, model: string) {
  return Object.keys(mapping).find(key => sameModel(key, model))
}
function clonePricing(pricing: ChannelModelPricing): ChannelModelPricing {
  return { ...pricing, models: [...pricing.models], intervals: pricing.intervals ? pricing.intervals.map(interval => ({ ...interval })) : [], time_pricing: pricing.time_pricing ? { ...pricing.time_pricing, periods: pricing.time_pricing.periods.map(period => ({ ...period })) } : null }
}
function operationWouldChange(channel: Channel): boolean {
  const source = sourceModel.value
  const pricing = channel.model_pricing.filter(item => item.platform === platform.value)
  if (operation.value === 'route') {
    const mapping = channel.model_mapping?.[platform.value] || {}
    const key = findMappingKey(mapping, source)
    return !key || mapping[key] !== targetModel.value
  }
  if (operation.value === 'add') return pricing.length > 0 && !pricing.some(item => item.models.some(model => sameModel(model, source)))
  return pricing.some(item => item.models.some(model => sameModel(model, source))) || !!channel.model_mapping?.[platform.value]?.[source]
}
function buildUpdate(channel: Channel) {
  const pricing = channel.model_pricing.map(clonePricing)
  const mapping: Record<string, Record<string, string>> = Object.fromEntries(Object.entries(channel.model_mapping || {}).map(([key, value]) => [key, { ...value }]))
  const platformPricing = pricing.filter(item => item.platform === platform.value)
  if (operation.value === 'add') {
    const target = platformPricing[0]
    if (!target || target.models.some(model => sameModel(model, sourceModel.value))) return null
    target.models.push(sourceModel.value)
  } else if (operation.value === 'remove') {
    for (let index = pricing.length - 1; index >= 0; index--) {
      if (pricing[index].platform !== platform.value) continue
      pricing[index].models = pricing[index].models.filter(model => !sameModel(model, sourceModel.value))
      if (!pricing[index].models.length) pricing.splice(index, 1)
    }
    if (mapping[platform.value]) {
      const key = findMappingKey(mapping[platform.value], sourceModel.value)
      if (key) delete mapping[platform.value][key]
      if (!Object.keys(mapping[platform.value]).length) delete mapping[platform.value]
    }
  } else {
    mapping[platform.value] ||= {}
    const key = findMappingKey(mapping[platform.value], sourceModel.value) || sourceModel.value
    if (mapping[platform.value][key] === targetModel.value) return null
    mapping[platform.value][key] = targetModel.value
  }
  return { model_pricing: pricing, model_mapping: mapping, ...(operation.value === 'remove' && enforceRestriction.value ? { restrict_models: true } : {}) }
}
async function loadChannels() {
  loading.value = true; error.value = ''
  try {
    const response = await adminAPI.channels.list(1, 1000, { sort_by: 'name', sort_order: 'asc' })
    channels.value = response.items || []
    selectedChannelIds.value = new Set(channels.value.map(channel => channel.id))
  } catch (err) {
    error.value = extractApiErrorMessage(err, text('加载渠道失败', 'Failed to load channels'))
  } finally { loading.value = false }
}
async function applyOperation() {
  if (!canApply.value || applying.value) return
  applying.value = true; error.value = ''; notice.value = ''
  const updates = affectedChannels.value.map(channel => ({ channel, request: buildUpdate(channel) })).filter(item => item.request)
  try {
    const results = await Promise.allSettled(updates.map(item => adminAPI.channels.update(item.channel.id, item.request!)))
    const failed = results.filter(result => result.status === 'rejected')
    if (failed.length) throw failed[0].status === 'rejected' ? failed[0].reason : new Error('Update failed')
    await loadChannels()
    notice.value = text(`已更新 ${updates.length} 个渠道`, `Updated ${updates.length} channel(s)`)
    appStore.showSuccess(notice.value)
  } catch (err) {
    error.value = extractApiErrorMessage(err, text('批量更新失败，部分渠道可能已更新，请刷新确认。', 'Bulk update failed; some channels may have been updated. Refresh to confirm.'))
  } finally { applying.value = false }
}
onMounted(loadChannels)
</script>
