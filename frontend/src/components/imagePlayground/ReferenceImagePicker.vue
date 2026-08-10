<template>
  <div
    data-test="reference-image-picker"
    class="flex h-[58px] max-w-full flex-none items-center gap-1.5 overflow-x-auto rounded-lg border border-gray-300 bg-white p-1.5 shadow-sm transition-colors dark:border-dark-600 dark:bg-dark-800 sm:max-w-[250px]"
    :class="dragActive ? 'border-gray-500 bg-gray-50 ring-2 ring-gray-900/10 dark:border-dark-400 dark:bg-dark-700' : ''"
    :title="t('imagePlayground.references.title')"
    @dragenter.prevent="dragActive = true"
    @dragover.prevent="dragActive = true"
    @dragleave.prevent="handleDragLeave"
    @drop.prevent="handleDrop"
  >
    <button
      v-if="files.length < maxFiles"
      type="button"
      class="relative flex h-11 w-11 flex-none items-center justify-center rounded-md border border-dashed border-gray-300 bg-gray-50 text-gray-500 transition-colors hover:border-gray-500 hover:bg-gray-100 hover:text-gray-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-gray-900/20 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:bg-dark-700 dark:hover:text-white"
      :title="t('imagePlayground.references.add')"
      @click="inputRef?.click()"
    >
      <Icon name="upload" size="sm" />
      <span class="sr-only">{{ t('imagePlayground.references.add') }}</span>
      <span class="absolute -right-1 -top-1 rounded-full border border-white bg-gray-700 px-1 text-[9px] leading-4 tabular-nums text-white dark:border-dark-800 dark:bg-gray-200 dark:text-gray-900">
        {{ files.length }}/{{ maxFiles }}
      </span>
    </button>

    <div
      v-for="preview in previews"
      :key="preview.key"
      class="group relative h-11 w-11 flex-none overflow-hidden rounded-md border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-900"
    >
      <img :src="preview.url" :alt="preview.file.name" class="h-full w-full object-cover" />
      <button
        type="button"
        class="absolute right-0.5 top-0.5 flex h-5 w-5 items-center justify-center rounded bg-black/75 text-white opacity-100 shadow-sm transition hover:bg-black sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
        :title="t('imagePlayground.references.remove')"
        @click="removeFile(preview.index)"
      >
        <Icon name="x" size="xs" />
      </button>
    </div>

    <button
      v-if="files.length > 0"
      type="button"
      class="flex h-8 w-8 flex-none items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-800 dark:text-gray-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
      :title="t('imagePlayground.references.clear')"
      @click="emit('update:files', [])"
    >
      <Icon name="trash" size="xs" />
      <span class="sr-only">{{ t('imagePlayground.references.clear') }}</span>
    </button>

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
