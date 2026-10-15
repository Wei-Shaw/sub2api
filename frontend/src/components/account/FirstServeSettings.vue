<template>
  <section ref="panel" class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="first-serve-settings">
    <div class="flex items-center justify-between gap-4">
      <div>
        <p class="input-label mb-0">{{ t(`${prefix}.mode`) }}</p>
        <p class="input-hint">{{ t(`${prefix}.modeHint`) }}</p>
      </div>
      <button
        type="button"
        role="switch"
        :aria-checked="enabled"
        :aria-label="t(`${prefix}.mode`)"
        data-testid="first-serve-toggle"
        class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
        :class="enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
        @click="emit('update:enabled', !enabled)"
      >
        <span class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition" :class="enabled ? 'translate-x-5' : 'translate-x-0'" />
      </button>
    </div>
    <template v-if="enabled">
      <p v-if="issue" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ issueText }}</p>
      <p v-else-if="!proxyGroupId" role="alert" class="text-sm text-amber-700 dark:text-amber-400">{{ t(`${prefix}.groupPending`, { name: accountName }) }}</p>
      <p v-else-if="availableCount < 2" role="status" class="text-xs text-amber-700 dark:text-amber-400">{{ t(`${prefix}.fewAvailable`, { name: accountName, count: availableCount }) }}</p>
      <details
        ref="configPanel"
        :open="expanded"
        class="rounded-xl border border-primary-200 bg-primary-50/40 p-4 dark:border-primary-800 dark:bg-primary-950/20"
        data-testid="first-serve-config"
        @toggle="expanded = ($event.target as HTMLDetailsElement).open"
        @invalid.capture="open"
      >
        <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t(`${prefix}.title`) }}</summary>
        <div class="mt-4 space-y-4">
          <p class="input-hint">{{ t(`${prefix}.hint`) }}</p>
          <label class="block">
            <span class="input-label">{{ t(`${prefix}.reuseScope`) }}</span>
            <select :value="modelValue.reuse_scope" class="input" data-testid="first-serve-scope" @change="setScope(($event.target as HTMLSelectElement).value)">
              <option value="session">{{ t(`${prefix}.scopeSession`) }}</option>
              <option value="account">{{ t(`${prefix}.scopeAccount`) }}</option>
            </select>
            <span class="input-hint">{{ t(`${prefix}.scopeHint`) }}</span>
          </label>
          <div class="grid gap-4 sm:grid-cols-2">
            <label v-for="field in firstServeFields" :key="field.key" class="block">
              <span class="input-label">{{ t(`${prefix}.${field.key}`) }}</span>
              <input :value="modelValue[field.key]" :data-testid="`first-serve-${field.key}`" type="number" class="input" required :min="field.min" :max="field.max" step="1" @input="set(field.key, Number(($event.target as HTMLInputElement).value))" />
              <span class="input-hint">{{ t(`${prefix}.range`, { min: field.min, max: field.max }) }}</span>
            </label>
          </div>
          <label class="block">
            <span class="input-label">{{ t(`${prefix}.group`) }}</span>
            <select :value="proxyGroupId ?? ''" class="input" data-testid="first-serve-group" @change="emit('update:proxyGroupId', Number(($event.target as HTMLSelectElement).value) || null)">
              <option value="">{{ t(`${prefix}.chooseGroup`) }}</option>
              <option v-for="group in proxyGroups" :key="group.id" :value="group.id">{{ group.name }} · {{ group.available_member_count }}/{{ group.member_count }}</option>
            </select>
          </label>
          <div class="space-y-2">
            <label class="block">
              <span class="input-label">{{ t(`${prefix}.proxies`) }}</span>
              <select :value="modelValue.proxy_mode" class="input" data-testid="first-serve-proxy-mode" @change="setMode(($event.target as HTMLSelectElement).value)">
                <option value="all">{{ t(`${prefix}.all`) }}</option>
                <option value="selected">{{ t(`${prefix}.selected`) }}</option>
              </select>
            </label>
            <p class="input-hint">{{ t(`${prefix}.proxyHint`) }}</p>
            <div v-if="modelValue.proxy_mode === 'selected'" class="max-h-52 space-y-2 overflow-y-auto rounded-lg border border-gray-200 p-3 dark:border-dark-600">
              <p v-if="!members.length" class="text-xs text-gray-500">{{ t(`${prefix}.noMembers`) }}</p>
              <label v-for="proxy in members" :key="proxy.id" class="flex cursor-pointer items-start gap-2 text-sm">
                <input type="checkbox" class="mt-1" :data-testid="`first-serve-proxy-${proxy.id}`" :checked="modelValue.proxy_ids.includes(proxy.id)" @change="toggle(proxy.id)" />
                <span class="min-w-0 break-all">
                  {{ proxy.name }} · {{ proxy.host }}:{{ proxy.port }}
                  <span class="block text-xs text-gray-500">{{ t(`${prefix}.exitIP`, { ip: proxy.ip_address || '—' }) }}<span v-if="!available(proxy)"> · {{ t(`${prefix}.unavailable`) }}</span></span>
                </span>
              </label>
              <div v-for="id in missing" :key="id" class="flex items-center justify-between gap-2 text-xs text-red-600 dark:text-red-400">
                <span>{{ t(`${prefix}.missingProxy`, { id }) }}</span>
                <button type="button" class="underline" @click="toggle(id)">{{ t('common.remove') }}</button>
              </div>
            </div>
          </div>
          <slot v-if="expanded" />
        </div>
      </details>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Proxy, ProxyGroup } from '@/types'
import { firstServeFields, firstServeIssue, type FirstServeConfig } from '@/utils/firstServe'

const props = defineProps<{
  enabled: boolean
  modelValue: FirstServeConfig
  proxyGroupId: number | null
  accountName: string
  proxies: Proxy[]
  proxyGroups: ProxyGroup[]
}>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:modelValue': [value: FirstServeConfig]
  'update:proxyGroupId': [value: number | null]
}>()
const { t } = useI18n()
const prefix = 'admin.accounts.openai.firstServeSettings'
const panel = ref<HTMLElement>()
const configPanel = ref<HTMLDetailsElement>()
const expanded = ref(false)
watch(() => props.enabled, () => { expanded.value = false })
watch([() => props.enabled, () => props.proxyGroups], () => {
  if (!props.enabled || props.proxyGroupId) return
  const defaultGroup = props.proxyGroups.find(item => item.status === 'active' && item.available_member_count >= 2
    && (props.modelValue.proxy_mode === 'all' || props.modelValue.proxy_ids.every(id => item.proxy_ids.includes(id))))
  if (!defaultGroup) {
    expanded.value = true
    return
  }
  emit('update:proxyGroupId', defaultGroup.id)
}, { immediate: true })
const group = computed(() => props.proxyGroups.find(item => item.id === props.proxyGroupId))
const members = computed(() => props.proxies.filter(proxy => group.value?.proxy_ids.includes(proxy.id)))
const missing = computed(() => props.modelValue.proxy_ids.filter(id => !members.value.some(proxy => proxy.id === id)))
const available = (proxy: Proxy) => group.value?.status === 'active' && proxy.status === 'active' && (!proxy.expires_at || new Date(proxy.expires_at).getTime() > Date.now())
const availableCount = computed(() => members.value.filter(proxy => available(proxy) && (props.modelValue.proxy_mode === 'all' || props.modelValue.proxy_ids.includes(proxy.id))).length)
const issue = computed(() => firstServeIssue(props.modelValue, props.proxyGroupId, props.proxyGroups))
const issueText = computed(() => {
  if (!issue.value) return ''
  const params: Record<string, unknown> = { ...issue.value.params, name: props.accountName }
  if ('field' in params) params.field = t(`${prefix}.${params.field}`)
  return t(`${prefix}.${issue.value.key}`, params)
})

function set<K extends keyof FirstServeConfig>(key: K, value: FirstServeConfig[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
function setScope(value: string) {
  set('reuse_scope', value === 'account' ? 'account' : 'session')
}
function setMode(value: string) {
  emit('update:modelValue', { ...props.modelValue, proxy_mode: value === 'selected' ? 'selected' : 'all', proxy_ids: [] })
}
function toggle(id: number) {
  const ids = props.modelValue.proxy_ids
  set('proxy_ids', ids.includes(id) ? ids.filter(value => value !== id) : [...ids, id])
}
function open() {
  expanded.value = true
  // Open immediately so native form validation can focus collapsed inputs.
  if (configPanel.value) configPanel.value.open = true
  panel.value?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
}
function validate() {
  if (!props.enabled || !issue.value) return true
  open()
  return false
}
defineExpose({ validate })
</script>
