<template>
  <div>
    <div class="mb-2 flex items-center justify-between gap-2">
      <label class="input-label mb-0">{{ t('admin.accounts.opencodeGo.protocolRules.title') }}</label>
      <button
        type="button"
        class="text-xs text-primary-600 hover:text-primary-700 dark:text-primary-400"
        @click="restoreDefaults"
      >
        {{ t('admin.accounts.opencodeGo.protocolRules.restoreDefaults') }}
      </button>
    </div>
    <p class="input-hint mb-2">{{ t('admin.accounts.opencodeGo.protocolRules.hint') }}</p>
    <div v-if="rows.length > 0" class="mb-2 space-y-2">
      <div v-for="(row, index) in rows" :key="getRowKey(row)">
        <div class="flex items-center gap-2">
          <input
            v-model="row.pattern"
            type="text"
            class="input flex-1 font-mono text-sm"
            :placeholder="t('admin.accounts.opencodeGo.protocolRules.patternPlaceholder')"
            :data-testid="`opencode-go-protocol-pattern-${index}`"
          />
          <select
            :value="row.protocol"
            class="input w-44 shrink-0"
            :data-testid="`opencode-go-protocol-select-${index}`"
            @change="setPrimaryProtocol(row, ($event.target as HTMLSelectElement).value as CnNativeApiProtocol)"
          >
            <option v-for="option in protocolOptions" :key="option.value" :value="option.value">
              {{ t(`admin.accounts.cnProviders.apiProtocol.${option.labelKey}`) }}
            </option>
          </select>
          <button
            type="button"
            class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
            :aria-label="t('admin.accounts.opencodeGo.protocolRules.remove')"
            @click="removeRow(index)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
        <div
          v-if="protocolOptions.length > 1"
          class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400"
          :title="t('admin.accounts.opencodeGo.protocolRules.alsoSupportsHint')"
        >
          <span>{{ t('admin.accounts.opencodeGo.protocolRules.alsoSupports') }}</span>
          <button
            v-for="option in protocolOptions.filter(item => item.value !== row.protocol)"
            :key="option.value"
            type="button"
            class="rounded-full border px-2 py-0.5 transition-colors"
            :class="hasExtraProtocol(row, option.value)
              ? 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
              : 'border-gray-200 text-gray-500 hover:border-gray-300 dark:border-dark-600 dark:text-gray-400 dark:hover:border-dark-500'"
            :aria-pressed="hasExtraProtocol(row, option.value)"
            :data-testid="`opencode-go-protocol-extra-${index}-${option.value}`"
            @click="toggleExtraProtocol(row, option.value)"
          >
            {{ t(`admin.accounts.cnProviders.apiProtocol.${option.labelKey}`) }}
          </button>
        </div>
      </div>
    </div>
    <div
      class="mb-2 flex items-center gap-2 rounded-lg border border-dashed border-gray-200 bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-800/60 dark:text-gray-400"
      data-testid="opencode-go-protocol-fallback"
    >
      <span class="flex-1 font-mono">*</span>
      <span>{{ t(`admin.accounts.opencodeGo.protocolRules.${hasModelCatalog ? 'catalogFallback' : 'fallback'}`) }}</span>
    </div>
    <button
      type="button"
      class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
      data-testid="opencode-go-protocol-add-rule"
      @click="addRow"
    >
      {{ t('admin.accounts.opencodeGo.protocolRules.add') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import {
  cloneOpenCodeGoProtocolRules,
  defaultProviderProtocolRules,
  providerHasModelCatalog,
  providerNativeProtocols,
  type CnNativeApiProtocol,
  type OpenCodeGoProtocolRule
} from '@/components/account/credentialsBuilder'

// 按模型分流的供应商（OpenCode 与后端新登记的聚合平台）的分流规则编辑器；
// 默认规则与可选协议来自平台清单中该供应商 / 接入模式的 profile。
const props = withDefaults(defineProps<{
  rows: OpenCodeGoProtocolRule[]
  platform?: string
  plan?: string
}>(), {
  platform: 'opencode_go',
  plan: 'go'
})

const emit = defineEmits<{
  (e: 'update:rows', rows: OpenCodeGoProtocolRule[]): void
}>()

const { t } = useI18n()
const getRowKey = createStableObjectKeyResolver<OpenCodeGoProtocolRule>('opencode-go-protocol-rule')

const addRow = () => {
  emit('update:rows', [...props.rows, { pattern: '', protocol: 'chat_completions' }])
}

const removeRow = (index: number) => {
  emit('update:rows', props.rows.filter((_, i) => i !== index))
}

const PROTOCOL_ORDER: Array<{ value: CnNativeApiProtocol; labelKey: string }> = [
  { value: 'chat_completions', labelKey: 'chatCompletions' },
  { value: 'responses', labelKey: 'responses' },
  { value: 'anthropic', labelKey: 'anthropic' }
]

// 仅列出该接入模式提供原生端点的协议；已有规则使用的协议保留，避免编辑时丢值。
const protocolOptions = computed(() => {
  const supported = new Set<CnNativeApiProtocol>(providerNativeProtocols(props.platform, props.plan))
  for (const row of props.rows) {
    supported.add(row.protocol)
    for (const protocol of row.extraProtocols ?? []) supported.add(protocol)
  }
  const options = PROTOCOL_ORDER.filter(option => supported.has(option.value))
  return options.length > 0 ? options : PROTOCOL_ORDER
})

const hasModelCatalog = computed(() => providerHasModelCatalog(props.platform))

// 规则的协议集合：首选协议 + 也支持的协议；入站协议在集合中时同协议直通。
const hasExtraProtocol = (row: OpenCodeGoProtocolRule, protocol: CnNativeApiProtocol) =>
  row.extraProtocols?.includes(protocol) ?? false

const setExtraProtocols = (row: OpenCodeGoProtocolRule, extra: CnNativeApiProtocol[]) => {
  if (extra.length > 0) row.extraProtocols = extra
  else delete row.extraProtocols
}

const toggleExtraProtocol = (row: OpenCodeGoProtocolRule, protocol: CnNativeApiProtocol) => {
  const extra = row.extraProtocols ?? []
  setExtraProtocols(
    row,
    extra.includes(protocol)
      ? extra.filter(item => item !== protocol)
      : PROTOCOL_ORDER.map(option => option.value).filter(value => value === protocol || extra.includes(value))
  )
}

// 切换首选协议：新协议原本就在“也支持”中时只调换顺序（原首选转入“也支持”），
// 否则替换原首选协议。
const setPrimaryProtocol = (row: OpenCodeGoProtocolRule, protocol: CnNativeApiProtocol) => {
  const previous = row.protocol
  if (protocol === previous) return
  const extra = row.extraProtocols ?? []
  row.protocol = protocol
  if (!extra.includes(protocol)) return
  const set = new Set<CnNativeApiProtocol>([...extra.filter(item => item !== protocol), previous])
  setExtraProtocols(row, PROTOCOL_ORDER.map(option => option.value).filter(value => set.has(value)))
}

const restoreDefaults = () => {
  emit('update:rows', cloneOpenCodeGoProtocolRules(defaultProviderProtocolRules(props.platform, props.plan)))
}
</script>
