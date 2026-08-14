<template>
  <AppLayout>
    <ModelSquareBackground :is-dark='isDark' />

    <div
      class='relative z-10 min-h-screen transition-colors duration-500'
      :class='isDark ? "dark bg-[#0a0c10]" : "bg-slate-50"'
    >
      <div class='mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6 lg:px-8'>
        <ModelSquareHeader
          :search='search'
          :loading='loading'
          :is-dark='isDark'
          @update:search='setSearch'
          @refresh='loadModels'
          @toggle-dark='isDark = !isDark'
        />

        <ModelSquarePlatformFilter
          :model-value='platform'
          :platforms='platforms'
          @update:model-value='setPlatform'
        />

        <ModelSquareHint />

        <ModelSquareLoading v-if='loading' />
        <ModelSquareEmpty v-else-if='filteredModels.length === 0' />

        <div v-else class='grid grid-cols-1 lg:grid-cols-[320px_1fr] gap-8 items-start'>
          <ModelSquareModelIndex
            v-model='activeModelKey'
            :models='filteredModels'
            :search='debouncedSearch'
          />
          <div class='space-y-8'>
            <ModelSquareModelCard
              v-for='model in filteredModels'
              :key='model.key'
              :model='model'
              :user-group-rates='userGroupRates'
            />
          </div>
        </div>
      </div>

      <button
        v-show='showBackToTop'
        type='button'
        class='fixed bottom-8 right-8 z-50 flex items-center gap-2 rounded-2xl bg-indigo-500/90 px-4 py-3 text-sm font-semibold text-white shadow-lg shadow-indigo-500/30 backdrop-blur transition-all hover:scale-105 hover:bg-indigo-500 active:scale-95'
        aria-label='返回顶部'
        @click='scrollToTop'
      >
        <Icon name='arrowUp' size='sm' />
        <span class='hidden sm:inline'>返回顶部</span>
      </button>
    </div>
  </AppLayout>
</template>

<script setup lang='ts'>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useModelSquare } from '@/features/model-square/composables/useModelSquare'
import { useModelSquareFilters } from '@/features/model-square/composables/useModelSquareFilters'
import { useModelSquareSearch } from '@/features/model-square/composables/useModelSquareSearch'
import ModelSquareBackground from '@/features/model-square/components/ModelSquareBackground.vue'
import ModelSquareEmpty from '@/features/model-square/components/ModelSquareEmpty.vue'
import ModelSquareHeader from '@/features/model-square/components/v2/ModelSquareHeader.vue'
import ModelSquareHint from '@/features/model-square/components/ModelSquareHint.vue'
import ModelSquareLoading from '@/features/model-square/components/ModelSquareLoading.vue'
import ModelSquarePlatformFilter from '@/features/model-square/components/ModelSquarePlatformFilter.vue'
import ModelSquareModelCard from '@/features/model-square/components/v2/ModelSquareModelCard.vue'
import ModelSquareModelIndex from '@/features/model-square/components/v2/ModelSquareModelIndex.vue'

const { loading, userGroupRates, platforms, modelGroups, loadModels } = useModelSquare()
const { search, debouncedSearch, setSearch } = useModelSquareSearch()
const { platform, setPlatform, filteredModels } = useModelSquareFilters({
  modelGroups,
  search: debouncedSearch,
})

const activeModelKey = ref<string | null>(null)

// 局部暗色：只影响本页容器，不污染 html 根元素
const isDark = ref(true)

const showBackToTop = ref(false)

function onScroll() {
  showBackToTop.value = (window.scrollY || document.documentElement.scrollTop) > 300
}

function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => {
  const saved = localStorage.getItem('model-square-theme')
  if (saved === 'light' || saved === 'dark') {
    isDark.value = saved === 'dark'
  }
  window.addEventListener('scroll', onScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
})

watch(isDark, (value) => {
  localStorage.setItem('model-square-theme', value ? 'dark' : 'light')
})
</script>
