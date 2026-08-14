<template>
  <aside
    ref='indexRef'
    tabindex='0'
    class='hidden lg:flex sticky top-8 h-fit max-h-[calc(100vh-4rem)] w-full flex-col overflow-hidden rounded-3xl border border-gray-200 dark:border-dark-700/60 bg-white/80 dark:bg-dark-900/60 backdrop-blur-xl shadow-2xl shadow-black/10 dark:shadow-black/20 focus:outline-none focus:ring-2 focus:ring-indigo-500/30'
    @keydown='onKeydown'
  >
    <div class='px-5 py-4 border-b border-white/10 dark:border-dark-700/60'>
      <h2 class='text-sm font-black uppercase tracking-[0.25em] text-gray-500 dark:text-dark-400'>模型索引</h2>
      <p class='mt-1 text-sm text-gray-600 dark:text-dark-400'>↑↓ 选择 · Enter 跳转 · 共 {{ models.length }} 个</p>
    </div>
    <div class='overflow-y-auto p-3 space-y-1 no-scrollbar'>
      <template v-for='group in groupedModels' :key='group.platform'>
        <div class='px-3 pt-4 pb-2 flex items-center gap-2 text-xs font-black uppercase tracking-[0.2em] text-gray-500 dark:text-dark-500'>
          <PlatformIcon :platform='group.platform' size='sm' />
          <span>{{ platformLabel(group.platform) }}</span>
        </div>
        <button
          v-for='item in group.items'
          :key='item.model.key'
          type='button'
          class='w-full text-left rounded-2xl px-4 py-3.5 text-base font-semibold transition-all duration-200 group'
          :class='modelValue === item.model.key ? activeClass : inactiveClass'
          @click='selectModel(item.model.key, item.index)'
        >
          <span class='flex items-center gap-3'>
            <span class='h-2 w-2 rounded-full shrink-0' :class='dotClass(item.model.platform)'></span>
            <span class='block truncate' v-html='highlight(item.model.name)'></span>
          </span>
          <span class='mt-1.5 block text-sm font-mono uppercase tracking-wider opacity-50'>{{ item.model.channels.length }} 渠道</span>
        </button>
      </template>
      <p v-if='models.length === 0' class='px-4 py-8 text-center text-sm text-gray-500 dark:text-dark-500'>没有匹配的模型</p>
    </div>
  </aside>
</template>

<script setup lang='ts'>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { ModelSquareModel } from '../../types'
import { platformLabel, platformBadgeLightClass } from '@/utils/platformColors'

interface Props {
  models: ModelSquareModel[]
  modelValue: string | null
  search?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:modelValue': [value: string | null]
}>()

const activeClass = 'bg-gradient-to-r from-indigo-500/20 to-purple-500/20 text-indigo-900 dark:text-white shadow-lg shadow-indigo-500/10 border border-indigo-500/30'
const inactiveClass = 'text-gray-600 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800/60 border border-transparent'

const indexRef = ref<HTMLElement | null>(null)
const selectedIndex = ref(0)

const normalizedSearch = computed(() => props.search?.trim().toLowerCase() ?? '')

const groupedModels = computed(() => {
  const groups: { platform: string; items: { model: ModelSquareModel; index: number }[] }[] = []
  const map = new Map<string, { platform: string; items: { model: ModelSquareModel; index: number }[] }>()
  props.models.forEach((model, index) => {
    if (!map.has(model.platform)) {
      const group = { platform: model.platform, items: [] as { model: ModelSquareModel; index: number }[] }
      map.set(model.platform, group)
      groups.push(group)
    }
    map.get(model.platform)!.items.push({ model, index })
  })
  return groups
})

function dotClass(platform: string) {
  return platformBadgeLightClass(platform).split(' ')[0].replace('bg-', 'bg-').replace('/10', '') + ' ring-2 ring-black/10 dark:ring-white/20'
}

function highlight(name: string) {
  if (!normalizedSearch.value) return name
  const idx = name.toLowerCase().indexOf(normalizedSearch.value)
  if (idx === -1) return name
  const before = name.slice(0, idx)
  const match = name.slice(idx, idx + normalizedSearch.value.length)
  const after = name.slice(idx + normalizedSearch.value.length)
  return `${before}<mark class='rounded px-0.5 bg-amber-400/90 text-gray-900'>${match}</mark>${after}`
}

function selectModel(key: string, index?: number) {
  if (index != null) selectedIndex.value = index
  emit('update:modelValue', key)
  const el = document.getElementById('model-' + key)
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function focusSelected() {
  const buttons = indexRef.value?.querySelectorAll('button') ?? []
  const el = buttons[selectedIndex.value] as HTMLElement | undefined
  if (el) {
    el.focus()
    el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }
}

function onKeydown(e: KeyboardEvent) {
  if (!indexRef.value || props.models.length === 0) return
  const isInput = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement
  if (isInput) return

  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const step = e.key === 'ArrowDown' ? 1 : -1
    selectedIndex.value = (selectedIndex.value + step + props.models.length) % props.models.length
    focusSelected()
  } else if (e.key === 'Enter' && selectedIndex.value >= 0) {
    e.preventDefault()
    selectModel(props.models[selectedIndex.value].key, selectedIndex.value)
  }
}

let observer: IntersectionObserver | null = null

onMounted(() => {
  observer = new IntersectionObserver((entries) => {
    const visible = entries.filter((e) => e.isIntersecting)
    if (visible.length > 0) {
      const top = visible.sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0]
      const key = top.target.getAttribute('data-model-key')
      if (key) {
        emit('update:modelValue', key)
        const idx = props.models.findIndex((m) => m.key === key)
        if (idx !== -1) selectedIndex.value = idx
      }
    }
  }, { rootMargin: '-10% 0px -60% 0px', threshold: 0 })
  document.querySelectorAll('[data-model-key]').forEach((el) => observer?.observe(el))
})

onUnmounted(() => {
  if (observer) observer.disconnect()
})
</script>
