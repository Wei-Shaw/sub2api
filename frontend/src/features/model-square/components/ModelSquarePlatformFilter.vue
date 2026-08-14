<template>
  <div class='mb-8 flex flex-wrap gap-2 px-2'>
    <button
      v-for='item in platforms'
      :key='item'
      type='button'
      class='inline-flex items-center gap-2 px-5 py-2 rounded-xl text-xs font-black transition-all active:scale-95'
      :class='item === modelValue ? activeClass : inactiveClass'
      @click='$emit("update:modelValue", item)'
    >
      <PlatformIcon v-if='item !== "all"' :platform='item' size='xs' />
      <span>{{ item === 'all' ? '全部平台' : item.toUpperCase() }}</span>
    </button>
  </div>
</template>

<script setup lang='ts'>
import PlatformIcon from '@/components/common/PlatformIcon.vue'

interface Props {
  platforms: string[]
  modelValue: string
}

defineProps<Props>()

defineEmits<{
  'update:modelValue': [value: string]
}>()

const activeClass = 'bg-gray-900 text-white shadow-md dark:bg-white dark:text-gray-900'
const inactiveClass = 'bg-white/80 text-gray-500 border border-gray-100 hover:border-gray-200 dark:bg-dark-900/60 dark:text-dark-400 dark:border-dark-800'
</script>
