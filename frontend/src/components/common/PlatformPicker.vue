<template>
  <div class="flex flex-wrap gap-2">
    <label
      v-for="p in platformList"
      :key="p.platform"
      class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors"
      :class="modelValue === p.platform
        ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700'
        : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
    >
      <input
        type="checkbox"
        :checked="modelValue === p.platform"
        class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        @change="toggle(p.platform)"
      />
      <PlatformIcon :platform="p.platform" :icon-svg="p.icon_svg" size="xs" :style="modelValue === p.platform && p.theme_color ? { color: p.theme_color } : {}" />
      <span :style="p.theme_color ? { color: p.theme_color } : {}">{{ p.display_name }}</span>
    </label>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue"
import PlatformIcon from "@/components/common/PlatformIcon.vue"
import { usePlatforms } from "@/composables/usePlatforms"

const { platforms, fetchPlatforms } = usePlatforms()
onMounted(() => fetchPlatforms())

const FALLBACK_PLATFORMS = [
  { platform: "anthropic", display_name: "Anthropic", icon_svg: "", theme_color: "#ea580c", sort_order: 1 },
  { platform: "openai", display_name: "OpenAI", icon_svg: "", theme_color: "#10b981", sort_order: 2 },
  { platform: "gemini", display_name: "Gemini", icon_svg: "", theme_color: "#2563eb", sort_order: 3 },
  { platform: "antigravity", display_name: "Antigravity", icon_svg: "", theme_color: "#7c3aed", sort_order: 4 },
]

const platformList = computed(() => {
  const sorted = [...platforms.value].sort((a, b) => a.sort_order - b.sort_order)
  return sorted.length > 0 ? sorted : FALLBACK_PLATFORMS
})

const props = defineProps<{ modelValue: string | null | undefined }>()
const emit = defineEmits<{ (e: "update:modelValue", value: string | null): void }>()

function toggle(p: string) {
  emit("update:modelValue", props.modelValue === p ? null : p)
}
</script>