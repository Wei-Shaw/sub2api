<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { prettyJson, tryParseJsonBody } from '../utils/jsonFormat'
import JsonNode from './JsonNode.vue'

const props = defineProps<{
  title: string
  raw?: string | null
  emptyText?: string
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const sourceMode = ref(false)
const expanded = ref<Record<string, boolean>>({ root: true })
const stringExpanded = ref<Record<string, boolean>>({})
const arrayPage = ref<Record<string, number>>({})

const parsed = computed(() => tryParseJsonBody(props.raw))
const isTree = computed(() => !parsed.value.empty && typeof parsed.value.value === 'object' && parsed.value.value !== null)

watch(
  () => props.raw,
  () => {
    sourceMode.value = false
    expanded.value = { root: true }
    stringExpanded.value = {}
    arrayPage.value = {}
  }
)

function toggle(path: string) {
  expanded.value[path] = !(expanded.value[path] ?? path === 'root')
}
function toggleString(path: string) {
  stringExpanded.value[path] = !stringExpanded.value[path]
}
function setPage(path: string, page: number) {
  arrayPage.value[path] = Math.max(0, page)
}

async function copy() {
  const text =
    isTree.value && !sourceMode.value ? prettyJson(parsed.value.value) : parsed.value.text || String(parsed.value.value ?? '')
  await copyToClipboard(text, t('usage.diagnosis.copied'))
}
</script>

<template>
  <div class="rounded-xl border border-gray-200 dark:border-dark-700">
    <div class="flex flex-wrap items-center gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-700">
      <div class="text-sm font-medium text-gray-800 dark:text-gray-100">
        {{ title }}
        <span v-if="parsed.truncated" class="ml-2 text-xs text-amber-500">{{ t('usage.diagnosis.jsonTruncated') }}</span>
      </div>
      <div class="ml-auto flex items-center gap-2">
        <button
          v-if="isTree"
          type="button"
          class="rounded px-2 py-1 text-xs"
          :class="sourceMode ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-200'"
          @click="sourceMode = !sourceMode"
        >
          View source
        </button>
        <button type="button" class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-200" @click="copy">
          {{ t('usage.diagnosis.copy') }}
        </button>
      </div>
    </div>

    <div v-if="parsed.empty" class="p-4 text-sm text-gray-500 dark:text-gray-400">
      {{ emptyText || t('usage.diagnosis.emptyBody') }}
    </div>
    <pre
      v-else-if="!isTree || sourceMode"
      class="max-h-[480px] overflow-auto p-3 font-mono text-xs text-gray-800 dark:text-gray-100"
    >{{ isTree ? prettyJson(parsed.value) : parsed.text }}</pre>
    <div v-else class="max-h-[480px] overflow-auto p-3 font-mono text-xs">
      <JsonNode
        :value="parsed.value"
        path="root"
        :depth="0"
        :expanded="expanded"
        :string-expanded="stringExpanded"
        :array-page="arrayPage"
        @toggle="toggle"
        @toggle-string="toggleString"
        @page="setPage"
      />
    </div>
  </div>
</template>
