<template>
  <div :class="props.embedded ? 'space-y-4' : 'rounded border border-line bg-surface'">
    <div v-if="!props.embedded" class="border-b border-line px-4 py-3">
      <h2 class="text-sm font-semibold text-ink">
        {{ t('profile.avatar.title') }}
      </h2>
      <p class="mt-0.5 text-xs text-ink-tertiary">
        {{ t('profile.avatar.description') }}
      </p>
    </div>

    <div
      :class="props.embedded
        ? 'flex items-start gap-4'
        : 'flex flex-col gap-4 px-4 py-4 sm:flex-row sm:items-start'"
    >
      <!--
        The avatar keeps `rounded-full` — it is one of the two elements in the
        system allowed to. What it loses is the gradient fill and the coloured
        drop shadow; an empty avatar is now a hairline circle on a sunken
        ground with the initial in secondary ink.
      -->
      <div
        :class="[
          'flex shrink-0 items-center justify-center overflow-hidden rounded-full border border-line bg-surface-sunken font-semibold text-ink-secondary',
          props.embedded ? 'h-12 w-12 text-sm' : 'h-16 w-16 text-md',
        ]"
      >
        <img
          v-if="avatarPreviewUrl"
          data-testid="profile-avatar-preview"
          :src="avatarPreviewUrl"
          :alt="displayName"
          class="h-full w-full object-cover"
        >
        <span v-else aria-hidden="true">{{ avatarInitial }}</span>
      </div>

      <div class="min-w-0 flex-1 space-y-3">
        <div class="space-y-0.5">
          <p class="truncate text-sm font-semibold text-ink">
            {{ props.embedded ? t('profile.avatar.title') : displayName }}
          </p>
          <p class="text-xs text-ink-tertiary">
            {{ t('profile.avatar.uploadHint') }}
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <!--
            A `<label>` rather than a Button, because it has to own the hidden
            file input. It carries the outline treatment by hand so it matches
            the two real buttons beside it.
          -->
          <label
            class="inline-flex h-7 cursor-pointer items-center justify-center whitespace-nowrap rounded border border-line bg-surface px-2.5 text-xs font-medium text-ink transition-colors duration-fast ease-out hover:border-line-strong hover:bg-surface-hover"
          >
            <input
              data-testid="profile-avatar-file-input"
              type="file"
              accept="image/*"
              class="sr-only"
              @change="handleAvatarFileChange"
            >
            {{ t('profile.avatar.uploadAction') }}
          </label>

          <Button
            data-testid="profile-avatar-save"
            tone="accent"
            variant="solid"
            :loading="avatarSaving"
            :disabled="!avatarDraft"
            @click="handleAvatarSave"
          >
            {{ t('common.save') }}
          </Button>

          <Button
            data-testid="profile-avatar-delete"
            :disabled="avatarSaving"
            @click="handleAvatarDelete"
          >
            {{ t('common.delete') }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userAPI } from '@/api'
import Button from '@/components/common/Button.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = withDefaults(defineProps<{
  user: User | null
  embedded?: boolean
}>(), {
  embedded: false,
})

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const targetAvatarUploadBytes = 20 * 1024
const avatarScaleSteps = [1, 0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36]
const avatarQualitySteps = [0.92, 0.84, 0.76, 0.68, 0.6, 0.52, 0.44, 0.36]
const avatarDraft = ref('')
const avatarSaving = ref(false)

const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || t('profile.user'))
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')
const avatarPreviewUrl = computed(() => avatarDraft.value.trim() || props.user?.avatar_url?.trim() || '')

watch(
  () => props.user?.avatar_url,
  () => {
    avatarDraft.value = ''
  }
)

function normalizeUploadedAvatar(value: string): string | null {
  const normalized = value.trim()
  if (!normalized) {
    return null
  }

  if (!/^data:image\/[a-zA-Z0-9.+-]+;base64,/i.test(normalized)) {
    appStore.showError(t('profile.avatar.uploadRequired'))
    return null
  }

  return normalized
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(reader.error ?? new Error('avatar_read_failed'))
    reader.readAsDataURL(file)
  })
}

function loadImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(t('profile.avatar.readFailed')))
    image.src = dataURL
  })
}

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error(t('profile.avatar.compressFailed')))
        return
      }
      resolve(blob)
    }, type, quality)
  })
}

async function compressAvatarFile(file: File): Promise<File> {
  const sourceDataURL = await readFileAsDataURL(file)
  const image = await loadImage(sourceDataURL)
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    throw new Error(t('profile.avatar.compressFailed'))
  }

  for (const scale of avatarScaleSteps) {
    const width = Math.max(1, Math.round(image.naturalWidth * scale))
    const height = Math.max(1, Math.round(image.naturalHeight * scale))
    canvas.width = width
    canvas.height = height
    ctx.clearRect(0, 0, width, height)
    ctx.drawImage(image, 0, 0, width, height)

    for (const quality of avatarQualitySteps) {
      const blob = await canvasToBlob(canvas, 'image/webp', quality)
      if (blob.size <= targetAvatarUploadBytes) {
        const fileName = file.name.replace(/\.[^.]+$/, '') || 'avatar'
        return new File([blob], `${fileName}.webp`, { type: 'image/webp' })
      }
    }
  }

  throw new Error(t('profile.avatar.compressTooLarge'))
}

async function prepareAvatarUpload(file: File): Promise<File> {
  if (!file.type.startsWith('image/')) {
    throw new Error(t('profile.avatar.invalidType'))
  }
  if (file.type === 'image/gif') {
    if (file.size > targetAvatarUploadBytes) {
      throw new Error(t('profile.avatar.gifTooLarge'))
    }
    return file
  }
  if (file.size <= targetAvatarUploadBytes) {
    return file
  }
  return compressAvatarFile(file)
}

async function handleAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (input) {
    input.value = ''
  }
  if (!file) {
    return
  }

  try {
    const preparedFile = await prepareAvatarUpload(file)
    const dataURL = await readFileAsDataURL(preparedFile)
    const normalized = normalizeUploadedAvatar(dataURL)
    if (!normalized) {
      return
    }
    avatarDraft.value = normalized
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  }
}

async function handleAvatarSave() {
  const normalized = normalizeUploadedAvatar(avatarDraft.value)
  if (!normalized) {
    return
  }

  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: normalized })
    authStore.user = updated
    avatarDraft.value = updated.avatar_url?.trim() || ''
    appStore.showSuccess(t('profile.avatar.saveSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    avatarSaving.value = false
  }
}

async function handleAvatarDelete() {
  if (avatarSaving.value) {
    return
  }
  if (!avatarDraft.value.trim() && !props.user?.avatar_url?.trim()) {
    appStore.showError(t('profile.avatar.emptyDeleteHint'))
    return
  }

  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: '' })
    authStore.user = updated
    avatarDraft.value = ''
    appStore.showSuccess(t('profile.avatar.deleteSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    avatarSaving.value = false
  }
}
</script>
