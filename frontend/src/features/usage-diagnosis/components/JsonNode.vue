<script setup lang="ts">
import { computed } from 'vue'
import JsonNode from './JsonNode.vue'

const props = defineProps<{
  value: unknown
  path: string
  depth: number
  label?: string
  expanded: Record<string, boolean>
  stringExpanded: Record<string, boolean>
  arrayPage: Record<string, number>
}>()

const emit = defineEmits<{
  toggle: [path: string]
  toggleString: [path: string]
  page: [path: string, page: number]
}>()

const isArr = computed(() => Array.isArray(props.value))
const isObj = computed(() => props.value !== null && typeof props.value === 'object')
const open = computed(() => props.expanded[props.path] ?? props.depth < 1)
const entries = computed(() => {
  if (!isObj.value) return [] as Array<[string, unknown]>
  if (isArr.value) return (props.value as unknown[]).map((v, i) => [String(i), v] as [string, unknown])
  return Object.entries(props.value as Record<string, unknown>)
})
const pageSize = 20
const page = computed(() => props.arrayPage[props.path] || 0)
const pageEntries = computed(() => {
  if (!isArr.value) return entries.value
  const start = page.value * pageSize
  return entries.value.slice(start, start + pageSize)
})

function typeClass(v: unknown) {
  if (v === null) return 'text-gray-400'
  if (typeof v === 'string') return 'text-amber-300'
  if (typeof v === 'number') return 'text-emerald-300'
  if (typeof v === 'boolean') return 'text-purple-300'
  return 'text-sky-300'
}

function isDataImage(v: unknown) {
  return typeof v === 'string' && /^data:image\//i.test(v)
}
</script>

<template>
  <div class="my-0.5" :style="{ marginLeft: depth * 12 + 'px' }">
    <template v-if="isDataImage(value)">
      <span v-if="label" class="text-sky-300">"{{ label }}"</span>
      <span v-if="label" class="text-gray-500">: </span>
      <div class="mt-1 inline-block rounded-lg border border-dark-600 bg-dark-900 p-2">
        <img :src="String(value)" class="max-h-40 max-w-xs rounded" alt="image" />
        <div class="mt-1 text-[10px] text-gray-400">data:image…</div>
      </div>
    </template>
    <template v-else-if="isObj">
      <button type="button" class="text-left text-gray-300" @click="emit('toggle', path)">
        {{ open ? '▼' : '▶' }}
        <span v-if="label" class="text-sky-300">"{{ label }}"</span>
        <span v-if="label" class="text-gray-500">: </span>
        <span class="text-gray-400">{{ isArr ? `Array(${entries.length})` : `Object{${entries.length}}` }}</span>
      </button>
      <template v-if="open">
        <JsonNode
          v-for="([k, v]) in pageEntries"
          :key="path + '.' + k"
          :value="v"
          :path="path + '.' + k"
          :depth="depth + 1"
          :label="k"
          :expanded="expanded"
          :string-expanded="stringExpanded"
          :array-page="arrayPage"
          @toggle="emit('toggle', $event)"
          @toggle-string="emit('toggleString', $event)"
          @page="(p, n) => emit('page', p, n)"
        />
        <div v-if="isArr && entries.length > pageSize" class="my-1 text-[10px] text-gray-400">
          <button type="button" class="mr-2 underline disabled:opacity-40" :disabled="page <= 0" @click="emit('page', path, page - 1)">Prev</button>
          {{ page + 1 }}/{{ Math.ceil(entries.length / pageSize) }}
          <button type="button" class="ml-2 underline disabled:opacity-40" :disabled="(page + 1) * pageSize >= entries.length" @click="emit('page', path, page + 1)">Next</button>
        </div>
      </template>
    </template>
    <template v-else>
      <span v-if="label" class="text-sky-300">"{{ label }}"</span>
      <span v-if="label" class="text-gray-500">: </span>
      <span :class="typeClass(value)">
        <template v-if="typeof value === 'string'">
          "{{ (value.length > 160 && !stringExpanded[path]) ? value.slice(0, 160) + '…' : value }}"
          <button
            v-if="value.length > 160"
            type="button"
            class="ml-2 text-[10px] text-primary-400 underline"
            @click="emit('toggleString', path)"
          >
            {{ stringExpanded[path] ? 'collapse' : `len ${value.length}` }}
          </button>
        </template>
        <template v-else>{{ String(value) }}</template>
      </span>
    </template>
  </div>
</template>
