<template>
  <section aria-labelledby="prompt-policy-title" class="py-6">
    <div>
      <h2 id="prompt-policy-title" class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.policy.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.policy.description') }}</p>
    </div>

    <div class="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(260px,0.45fr)]">
      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700/60 dark:bg-dark-900/20 sm:p-5">
        <fieldset>
          <legend class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.policy.scope') }}</legend>
          <div class="mt-3 flex flex-wrap gap-5 text-sm text-gray-700 dark:text-dark-200">
            <label class="flex items-center gap-2">
              <input type="radio" name="prompt-audit-scope" :checked="draft.all_groups" @change="patch({ all_groups: true, group_ids: [] })" />
              {{ t('admin.promptAudit.policy.allGroups') }}
            </label>
            <label class="flex items-center gap-2">
              <input type="radio" name="prompt-audit-scope" :checked="!draft.all_groups" @change="patch({ all_groups: false })" />
              {{ t('admin.promptAudit.policy.selectedGroups') }}
            </label>
          </div>
        </fieldset>

        <div v-if="!draft.all_groups" class="mt-4">
          <label class="block text-sm text-gray-700 dark:text-dark-200">
            <span>{{ t('admin.promptAudit.policy.searchGroups') }}</span>
            <input v-model="groupSearch" type="search" class="input mt-1.5 w-full" :aria-label="t('admin.promptAudit.policy.searchGroups')" />
          </label>
          <div class="mt-3 max-h-52 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-dark-700">
            <label v-for="group in filteredGroups" :key="group.id" class="flex cursor-pointer items-center justify-between gap-3 rounded-md px-2 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-800">
              <span class="flex items-center gap-2 text-gray-800 dark:text-dark-100">
                <input type="checkbox" :checked="draft.group_ids.includes(group.id)" @change="toggleGroup(group.id)" />
                {{ group.name }}
              </span>
              <span class="text-xs text-gray-500 dark:text-dark-400">{{ group.platform }} · {{ group.status }}</span>
            </label>
            <p v-if="filteredGroups.length === 0" class="px-2 py-4 text-center text-sm text-gray-500">{{ t('admin.promptAudit.policy.noGroups') }}</p>
          </div>
          <div v-if="missingGroupIds.length" class="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
            {{ t('admin.promptAudit.policy.missingGroups') }}: {{ missingGroupIds.join(', ') }}
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.policy.selectedCount', { count: draft.group_ids.length }) }}</p>
        </div>

        <fieldset class="mt-5 border-t border-gray-100 pt-5 dark:border-dark-800">
          <legend class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.policy.keywordBlockingMode') }}</legend>
          <div class="mt-3 rounded-lg border border-primary-100 bg-primary-50/60 px-3 py-2.5 text-sm dark:border-primary-900/40 dark:bg-primary-900/10">
            <p class="font-medium text-primary-700 dark:text-primary-200">{{ keywordNoticeTitle }}</p>
            <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-dark-300">{{ keywordNoticeDescription }}</p>
          </div>
          <div class="mt-3 grid gap-2 sm:grid-cols-3">
            <button
              v-for="option in keywordModeOptions"
              :key="option.value"
              type="button"
              class="rounded-lg border p-3 text-left transition-colors"
              :class="draft.keyword_blocking_mode === option.value
                ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
              :data-test="`keyword-mode-${option.value}`"
              @click="selectKeywordMode(option.value)"
            >
              <div class="flex items-center justify-between gap-2">
                <span class="text-sm font-semibold">{{ option.label }}</span>
                <span
                  class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
                  :class="draft.keyword_blocking_mode === option.value
                    ? 'border-primary-500 bg-primary-500 text-white'
                    : 'border-gray-300 text-transparent dark:border-dark-500'"
                >
                  <span class="text-[10px]">✓</span>
                </span>
              </div>
              <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
            </button>
          </div>

          <div class="mt-4">
            <div class="mb-2 flex items-center justify-between gap-3">
              <label class="text-sm text-gray-700 dark:text-dark-200" for="prompt-audit-blocked-keywords">{{ t('admin.promptAudit.policy.blockedKeywords') }}</label>
              <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300">
                {{ t('admin.promptAudit.policy.blockedKeywordCount', { count: keywordCount }) }}
              </span>
            </div>
            <textarea
              id="prompt-audit-blocked-keywords"
              v-model="keywordText"
              rows="6"
              class="input min-h-32 resize-y font-mono text-sm"
              :placeholder="t('admin.promptAudit.policy.blockedKeywordsPlaceholder')"
              :disabled="draft.keyword_blocking_mode === 'ai_only'"
              :aria-label="t('admin.promptAudit.policy.blockedKeywords')"
              data-test="blocked-keywords"
            />
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.promptAudit.policy.blockedKeywordsLimit') }}</p>
          </div>
        </fieldset>

        <fieldset class="mt-5 border-t border-gray-100 pt-5 dark:border-dark-800">
          <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.policy.preHashCheck') }}</p>
                <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.promptAudit.policy.preHashCheckHint') }}</p>
              </div>
              <label class="inline-flex shrink-0 cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                <input
                  type="checkbox"
                  :checked="draft.pre_hash_check_enabled"
                  :aria-label="t('admin.promptAudit.policy.preHashCheck')"
                  @change="patch({ pre_hash_check_enabled: ($event.target as HTMLInputElement).checked })"
                />
                <span>{{ draft.pre_hash_check_enabled ? t('common.enabled') : t('common.disabled') }}</span>
              </label>
            </div>
            <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
              <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <p class="text-sm font-medium text-gray-800 dark:text-gray-100">
                  {{ t('admin.promptAudit.policy.flaggedHashCount', { count: props.runtime?.flagged_hash_count ?? 0 }) }}
                </p>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-300"
                  :disabled="(props.runtime?.flagged_hash_count ?? 0) === 0"
                  @click="emit('clear-hashes')"
                >
                  {{ t('admin.promptAudit.policy.clearFlaggedHashes') }}
                </button>
              </div>
              <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('admin.promptAudit.policy.flaggedHashHint') }}</p>
              <div class="mt-3 flex flex-col gap-2 sm:flex-row">
                <input
                  v-model.trim="promptHash"
                  type="text"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.promptAudit.policy.flaggedHashPlaceholder')"
                  :aria-label="t('admin.promptAudit.policy.flaggedHashPlaceholder')"
                />
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :disabled="!promptHashValid"
                  @click="deletePromptHash"
                >
                  {{ t('admin.promptAudit.policy.deleteFlaggedHash') }}
                </button>
              </div>
            </div>
          </div>
        </fieldset>

        <fieldset class="mt-5 border-t border-gray-100 pt-5 dark:border-dark-800">
          <legend class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.promptAudit.policy.scanners') }}</legend>
          <div class="mt-3 grid gap-2 sm:grid-cols-2">
            <label v-for="scanner in SCANNER_CATALOG" :key="scanner.id" class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-gray-700 hover:bg-gray-50 dark:text-dark-200 dark:hover:bg-dark-800">
              <input type="checkbox" :checked="draft.scanners.includes(scanner.id)" :aria-label="scannerLabel(scanner.id)" @change="toggleScanner(scanner.id)" />
              <span>{{ scannerLabel(scanner.id) }}</span>
            </label>
          </div>
        </fieldset>
      </div>

      <div class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-700/60 dark:bg-dark-900/20 sm:p-5">
        <label class="block text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.policy.workerCount') }}</span>
          <input :value="draft.worker_count" type="number" min="1" max="32" class="input mt-1.5 w-full" :aria-label="t('admin.promptAudit.policy.workerCount')" @input="patch({ worker_count: Number(($event.target as HTMLInputElement).value) })" />
        </label>
        <label class="block text-sm text-gray-700 dark:text-dark-200">
          <span>{{ t('admin.promptAudit.policy.queueCapacity') }}</span>
          <input :value="draft.queue_capacity" type="number" min="1" max="100000" class="input mt-1.5 w-full" :aria-label="t('admin.promptAudit.policy.queueCapacity')" @input="patch({ queue_capacity: Number(($event.target as HTMLInputElement).value) })" />
        </label>
        <div class="rounded-lg bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:bg-dark-900/50 dark:text-dark-300">
          <p class="font-medium text-gray-800 dark:text-dark-100">{{ t('admin.promptAudit.policy.strategy') }}</p>
          <p class="mt-1">priority · {{ t('admin.promptAudit.policy.strategyHint') }}</p>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PromptAuditDraft, PromptAuditGroup, PromptAuditRuntime } from '../types'
import { cloneData, PROMPT_KEYWORD_MAX, PROMPT_KEYWORD_MAX_RUNES, SCANNER_CATALOG } from '../viewModel'

const props = defineProps<{ draft: PromptAuditDraft; groups: PromptAuditGroup[]; runtime?: PromptAuditRuntime | null }>()
const emit = defineEmits<{
  (event: 'update:draft', value: PromptAuditDraft): void
  (event: 'delete-hash', value: string): void
  (event: 'clear-hashes'): void
}>()
const { t } = useI18n()
const groupSearch = ref('')
const promptHash = ref('')
const promptHashValid = computed(() => /^[a-fA-F0-9]{64}$/.test(promptHash.value.trim()))

const filteredGroups = computed(() => {
  const query = groupSearch.value.trim().toLowerCase()
  if (!query) return props.groups
  return props.groups.filter((group) => `${group.name} ${group.id} ${group.platform}`.toLowerCase().includes(query))
})
const knownGroupIds = computed(() => new Set(props.groups.map((group) => group.id)))
const missingGroupIds = computed(() => props.draft.group_ids.filter((id) => !knownGroupIds.value.has(id)))
const keywordModeOptions = computed(() => [
  { value: 'ai_only' as const, label: t('admin.promptAudit.policy.keywordModeAIOnly'), description: t('admin.promptAudit.policy.keywordModeAIOnlyDesc') },
  { value: 'keyword_only' as const, label: t('admin.promptAudit.policy.keywordModeKeywordOnly'), description: t('admin.promptAudit.policy.keywordModeKeywordOnlyDesc') },
  { value: 'keyword_and_ai' as const, label: t('admin.promptAudit.policy.keywordModeKeywordAndAI'), description: t('admin.promptAudit.policy.keywordModeKeywordAndAIDesc') },
])
const keywordText = computed({
  get: () => (props.draft.blocked_keywords ?? []).join('\n'),
  set: (value: string) => patch({ blocked_keywords: parseKeywords(value) }),
})
const keywordCount = computed(() => (props.draft.blocked_keywords ?? []).length)
const keywordNoticeTitle = computed(() => {
  if (props.draft.keyword_blocking_mode === 'keyword_only') return t('admin.promptAudit.policy.keywordModeKeywordOnlyNotice')
  if (props.draft.keyword_blocking_mode === 'keyword_and_ai') return t('admin.promptAudit.policy.keywordModeKeywordAndAINotice')
  return t('admin.promptAudit.policy.keywordModeAIOnlyNotice')
})
const keywordNoticeDescription = computed(() => {
  if (props.draft.keyword_blocking_mode === 'keyword_only') {
    return t(props.draft.keyword_blocking_enabled
      ? 'admin.promptAudit.policy.keywordOnlyBlockingHint'
      : 'admin.promptAudit.policy.keywordOnlyAsyncHint')
  }
  if (props.draft.keyword_blocking_mode === 'keyword_and_ai') {
    if (props.draft.keyword_blocking_enabled && props.draft.ai_blocking_enabled) {
      return t('admin.promptAudit.policy.keywordAndAIBothBlockingHint')
    }
    if (props.draft.keyword_blocking_enabled) {
      return t('admin.promptAudit.policy.keywordAndAIMixedHint')
    }
    if (props.draft.ai_blocking_enabled) {
      return t('admin.promptAudit.policy.keywordAndAIOnlyAIBlockingHint')
    }
    return t('admin.promptAudit.policy.keywordAndAIAsyncHint')
  }
  return t('admin.promptAudit.policy.keywordModeAIOnlyDesc')
})

function selectKeywordMode(value: PromptAuditDraft['keyword_blocking_mode']) {
  patch({
    keyword_blocking_mode: value,
    keyword_blocking_enabled: value === 'ai_only' ? false : props.draft.keyword_blocking_enabled,
    ai_blocking_enabled: value === 'keyword_only' ? false : props.draft.ai_blocking_enabled,
  })
}

function patch(value: Partial<PromptAuditDraft>) {
  emit('update:draft', { ...cloneData(props.draft), ...value })
}
function deletePromptHash() {
  if (!promptHashValid.value) return
  emit('delete-hash', promptHash.value.trim())
}
function parseKeywords(value: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const raw of value.split(/\r?\n/)) {
    const keyword = Array.from(raw.trim()).slice(0, PROMPT_KEYWORD_MAX_RUNES).join('')
    if (!keyword) continue
    const key = keyword.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(keyword)
    if (result.length >= PROMPT_KEYWORD_MAX) break
  }
  return result
}
function toggleGroup(id: number) {
  const selected = new Set(props.draft.group_ids)
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
  patch({ group_ids: [...selected].sort((a, b) => a - b) })
}
function toggleScanner(id: string) {
  const selected = new Set(props.draft.scanners)
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
  patch({ scanners: SCANNER_CATALOG.map((item) => item.id).filter((item) => selected.has(item)) })
}
function scannerLabel(id: string): string {
  return t(`admin.promptAudit.scanners.${id}`)
}
</script>
