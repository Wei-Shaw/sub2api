<template>
  <div
    class="border-b border-gray-100 px-3 py-3 dark:border-dark-800 sm:px-4"
    :class="dragActive ? 'bg-gray-50 dark:bg-dark-800/70' : ''"
    @dragenter.prevent="dragActive = true"
    @dragover.prevent="dragActive = true"
    @dragleave.prevent="handleDragLeave"
    @drop.prevent="handleDrop"
  >
    <div class="mb-2 flex items-center justify-between gap-3">
      <div class="flex min-w-0 items-center gap-2">
        <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('imagePlayground.references.title') }}</span>
        <span class="text-[11px] tabular-nums text-gray-400 dark:text-gray-500">{{ files.length }}/{{ maxFiles }}</span>
      </div>
      <button
        v-if="files.length > 0"
        type="button"
        class="text-xs text-gray-400 transition-colors hover:text-gray-800 dark:text-gray-500 dark:hover:text-gray-200"
        @click="emit('update:files', [])"
      >
        {{ t('imagePlayground.references.clear') }}
      </button>
    </div>

    <div class="flex min-w-0 gap-2 overflow-x-auto pb-0.5">
      <button
        v-if="files.length < maxFiles"
        type="button"
        class="flex h-16 w-16 flex-none flex-col items-center justify-center rounded-md border border-dashed border-gray-300 bg-gray-50 text-gray-500 transition-colors hover:border-gray-500 hover:bg-gray-100 hover:text-gray-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-900/20 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:bg-dark-700 dark:hover:text-white"
        :title="t('imagePlayground.references.add')"
        @click="inputRef?.click()"
      >
        <Icon name="upload" size="sm" />
        <span class="mt-1 text-[10px] leading-none">{{ t('imagePlayground.references.addShort') }}</span>
      </button>

      <div
        v-for="preview in previews"
        :key="preview.key"
        class="group relative h-16 w-16 flex-none overflow-hidden rounded-md border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800"
      >
        <img :src="preview.url" :alt="preview.file.name" class="h-full w-full object-cover" />
        <button
          type="button"
          class="absolute right-1 top-1 flex h-6 w-6 items-center justify-center rounded-md bg-black/70 text-white opacity-100 shadow-sm transition hover:bg-black sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
          :title="t('imagePlayground.references.remove')"
          @click="removeFile(preview.index)"
        >
          <Icon name="x" size="xs" />
        </button>
        <span class="absolute inset-x-0 bottom-0 truncate bg-black/60 px-1.5 py-1 text-[9px] text-white">
          {{ preview.file.name }}
        </span>
      </div>
    </div>

    <input
      ref="inputRef"
      type="file"
      class="hidden"
      multiple
      :accept="acceptedTypes.join(',')"
      @change="handleInput"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  files: File[]
  maxFiles?: number
  maxBytes?: number
  acceptedTypes?: string[]
}>(), {
  maxFiles: 4,
  maxBytes: 10 * 1024 * 1024,
  acceptedTypes: () => ['image/png', 'image/jpeg', 'image/webp'],
})

const emit = defineEmits<{
  'update:files': [files: File[]]
  error: [code: 'tooMany' | 'tooLarge' | 'invalidType', detail?: string]
}>()

const { t } = useI18n()
const inputRef = ref<HTMLInputElement | null>(null)
const dragActive = ref(false)
const previewURLs = ref<string[]>([])

const previews = computed(() => props.files.map((file, index) => ({
  file,
  index,
  key: `${file.name}-${file.size}-${file.lastModified}-${index}`,
  url: previewURLs.value[index] || '',
})))

watch(() => props.files, (files) => {
  revokePreviews()
  previewURLs.value = files.map((file) => URL.createObjectURL(file))
}, { immediate: true, deep: false })

function addFiles(incoming: File[]): void {
  const accepted: File[] = []
  for (const file of incoming) {
    if (!props.acceptedTypes.includes(file.type)) {
      emit('error', 'invalidType', file.name)
      continue
    }
    if (file.size > props.maxBytes) {
      emit('error', 'tooLarge', file.name)
      continue
    }
    accepted.push(file)
  }
  const remaining = Math.max(0, props.maxFiles - props.files.length)
  if (accepted.length > remaining) {
    emit('error', 'tooMany')
  }
  const next = [...props.files, ...accepted.slice(0, remaining)]
  emit('update:files', next)
}

function handleInput(event: Event): void {
  const input = event.target as HTMLInputElement
  addFiles(Array.from(input.files || []))
  input.value = ''
}

function handleDrop(event: DragEvent): void {
  dragActive.value = false
  addFiles(Array.from(event.dataTransfer?.files || []))
}

function handleDragLeave(event: DragEvent): void {
  const current = event.currentTarget as HTMLElement | null
  if (!current?.contains(event.relatedTarget as Node | null)) {
    dragActive.value = false
  }
}

function removeFile(index: number): void {
  emit('update:files', props.files.filter((_, fileIndex) => fileIndex !== index))
}

function revokePreviews(): void {
  previewURLs.value.forEach((url) => URL.revokeObjectURL(url))
  previewURLs.value = []
}

onBeforeUnmount(revokePreviews)
</script>
