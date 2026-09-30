<template>
  <details ref="panel" open class="rounded-lg border p-3 dark:border-dark-600">
    <summary class="cursor-pointer text-sm font-medium">{{ text('负责部门', 'Managed departments') }} · {{ text('已选', 'Selected') }} {{ selected.length }}</summary>
    <div v-if="selected.length" class="my-3 flex flex-wrap gap-2" data-testid="selected-departments">
      <button v-for="id in selected" :key="id" type="button" class="rounded-lg bg-primary-50 px-3 py-2 text-left text-sm text-primary-700 dark:bg-dark-700 dark:text-primary-300" :aria-label="`${text('定位', 'Locate')} ${pathLabel(id)}`" @click="locate(id)">{{ pathLabel(id) }} <span aria-hidden="true">↗</span></button>
    </div>
    <label class="my-3 block text-sm">{{ text('搜索部门名称或 ID', 'Search department name or ID') }}<input v-model="search" data-testid="manager-department-search" type="search" class="input mt-1" /></label>
    <div ref="tree" class="max-h-80 overflow-auto" data-testid="manager-departments">
      <div v-for="dept in visibleRows" :key="dept.id" :data-department-id="dept.id" :class="selected.includes(dept.id) ? 'rounded bg-primary-50 text-primary-700 dark:bg-dark-700 dark:text-primary-300' : ''" class="flex items-center gap-1 py-1 text-sm" :style="{ paddingLeft: `${dept.depth * 16}px` }">
        <button v-if="dept.hasChildren" type="button" class="shrink-0 rounded p-1" :aria-expanded="expanded.has(dept.id)" :aria-label="`${text('展开/收起权限部门', 'Expand/collapse managed department')} ${dept.name}`" @click="toggle(dept.id)">{{ expanded.has(dept.id) ? '▾' : '▸' }}</button>
        <span v-else class="w-6 shrink-0" />
        <label><input type="checkbox" :checked="selected.includes(dept.id)" @change="$emit('toggle', dept.id, ($event.target as HTMLInputElement).checked)" /> {{ dept.name }}</label>
      </div>
      <p v-if="!visibleRows.length" class="py-3 text-gray-500">{{ text('无匹配部门', 'No matching departments') }}</p>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DingTalkDepartment } from '@/api/dingtalk'
import { buildDingTalkDepartmentRows, dingTalkDepartmentPath } from '@/utils/dingtalkDepartments'
const props = defineProps<{ departments: DingTalkDepartment[]; selected: number[] }>()
defineEmits<{ (event: 'toggle', id: number, checked: boolean): void }>()
const { locale } = useI18n()
const text = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const search = ref('')
const expanded = ref(new Set<number>())
const panel = ref<HTMLDetailsElement | null>(null)
const tree = ref<HTMLElement | null>(null)
function pathLabel(id: number) { return dingTalkDepartmentPath(props.departments, id).map(d => d.name).join(' / ') || `#${id}` }
function expandPath(id: number) { dingTalkDepartmentPath(props.departments, id).slice(0, -1).forEach(d => expanded.value.add(d.id)) }
async function scrollTo(id: number) {
  await nextTick()
  tree.value?.querySelector<HTMLElement>(`[data-department-id="${id}"]`)?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })
}
async function locate(id: number) {
  search.value = ''
  if (panel.value) panel.value.open = true
  expandPath(id)
  await scrollTo(id)
}
watch(() => props.selected.map(id => dingTalkDepartmentPath(props.departments, id).map(d => d.id).join('/')).join(','), () => {
  props.selected.forEach(expandPath)
  if (props.selected[0] !== undefined) void scrollTo(props.selected[0])
}, { immediate: true })
const rows = computed(() => buildDingTalkDepartmentRows(props.departments))
const matches = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  if (!query) return null
  const keep = new Set<number>()
  const ancestors: number[] = []
  for (const d of rows.value) {
    ancestors.length = d.depth
    if (`${d.name} ${d.id}`.toLocaleLowerCase().includes(query)) { keep.add(d.id); ancestors.forEach(id => keep.add(id)) }
    ancestors.push(d.id)
  }
  return keep
})
watch(matches, ids => { ids?.forEach(id => expanded.value.add(id)) })
const visibleRows = computed(() => {
  let hiddenBelow = Infinity
  return rows.value.filter(d => {
    if (d.depth > hiddenBelow) return false
    hiddenBelow = expanded.value.has(d.id) ? Infinity : d.depth
    return !matches.value || matches.value.has(d.id)
  })
})
function toggle(id: number) { if (expanded.value.has(id)) expanded.value.delete(id); else expanded.value.add(id) }
</script>
