<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.avatar.title') REDACTEDREDACTED
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.avatar.description') REDACTEDREDACTED
      </p>
    </div>

    <div class="flex flex-col gap-5 px-6 py-6 sm:flex-row sm:items-start">
      <div
        class="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-3xl font-bold text-white shadow-lg shadow-primary-500/20"
      >
        <img
          v-if="avatarPreviewUrl"
          :src="avatarPreviewUrl"
          :alt="displayName"
          class="h-full w-full object-cover"
        >
        <span v-else>{{ avatarInitial REDACTEDREDACTED</span>
      </div>

      <div class="min-w-0 flex-1 space-y-4">
        <div class="space-y-1">
          <p class="text-sm font-medium text-gray-900 dark:text-white">
            {{ displayName REDACTEDREDACTED
          </p>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.avatar.uploadHint') REDACTEDREDACTED
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <label class="btn btn-secondary btn-sm cursor-pointer">
            <input
              data-testid="profile-avatar-file-input"
              type="file"
              accept="image/*"
              class="hidden"
              @change="handleAvatarFileChange"
            >
            {{ t('profile.avatar.uploadAction') REDACTEDREDACTED
          </label>

          <button
            data-testid="profile-avatar-save"
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="avatarSaving || !avatarDraft"
            @click="handleAvatarSave"
          >
            {{ t('common.save') REDACTEDREDACTED
          </button>

          <button
            data-testid="profile-avatar-delete"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="avatarSaving"
            @click="handleAvatarDelete"
          >
            {{ t('common.delete') REDACTEDREDACTED
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { userAPI REDACTED from '@/api'
import { useAppStore REDACTED from '@/stores/app'
import { useAuthStore REDACTED from '@/stores/auth'
import type { User REDACTED from '@/types'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'

const props = defineProps<{
  user: User | null
REDACTED>()

const { t REDACTED = useI18n()
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
  REDACTED
)

function normalizeUploadedAvatar(value: string): string | null {
  const normalized = value.trim()
  if (!normalized) {
    return null
  REDACTED

  if (!/^data:image\/[a-zA-Z0-9.+-]+;base64,/i.test(normalized)) {
    appStore.showError(t('profile.avatar.uploadRequired'))
    return null
  REDACTED

  return normalized
REDACTED

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(reader.error ?? new Error('avatar_read_failed'))
    reader.readAsDataURL(file)
  REDACTED)
REDACTED

function loadImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(t('profile.avatar.readFailed')))
    image.src = dataURL
  REDACTED)
REDACTED

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error(t('profile.avatar.compressFailed')))
        return
      REDACTED
      resolve(blob)
    REDACTED, type, quality)
  REDACTED)
REDACTED

async function compressAvatarFile(file: File): Promise<File> {
  const sourceDataURL = await readFileAsDataURL(file)
  const image = await loadImage(sourceDataURL)
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    throw new Error(t('profile.avatar.compressFailed'))
  REDACTED

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
        return new File([blob], `${fileNameREDACTED.webp`, { type: 'image/webp' REDACTED)
      REDACTED
    REDACTED
  REDACTED

  throw new Error(t('profile.avatar.compressTooLarge'))
REDACTED

async function prepareAvatarUpload(file: File): Promise<File> {
  if (!file.type.startsWith('image/')) {
    throw new Error(t('profile.avatar.invalidType'))
  REDACTED
  if (file.type === 'image/gif') {
    if (file.size > targetAvatarUploadBytes) {
      throw new Error(t('profile.avatar.gifTooLarge'))
    REDACTED
    return file
  REDACTED
  if (file.size <= targetAvatarUploadBytes) {
    return file
  REDACTED
  return compressAvatarFile(file)
REDACTED

async function handleAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (input) {
    input.value = ''
  REDACTED
  if (!file) {
    return
  REDACTED

  try {
    const preparedFile = await prepareAvatarUpload(file)
    const dataURL = await readFileAsDataURL(preparedFile)
    const normalized = normalizeUploadedAvatar(dataURL)
    if (!normalized) {
      return
    REDACTED
    avatarDraft.value = normalized
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  REDACTED
REDACTED

async function handleAvatarSave() {
  const normalized = normalizeUploadedAvatar(avatarDraft.value)
  if (!normalized) {
    return
  REDACTED

  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: normalized REDACTED)
    authStore.user = updated
    avatarDraft.value = updated.avatar_url?.trim() || ''
    appStore.showSuccess(t('profile.avatar.saveSuccess'))
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  REDACTED finally {
    avatarSaving.value = false
  REDACTED
REDACTED

async function handleAvatarDelete() {
  if (avatarSaving.value) {
    return
  REDACTED
  if (!avatarDraft.value.trim() && !props.user?.avatar_url?.trim()) {
    appStore.showError(t('profile.avatar.emptyDeleteHint'))
    return
  REDACTED

  avatarSaving.value = true
  try {
    const updated = await userAPI.updateProfile({ avatar_url: '' REDACTED)
    authStore.user = updated
    avatarDraft.value = ''
    appStore.showSuccess(t('profile.avatar.deleteSuccess'))
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  REDACTED finally {
    avatarSaving.value = false
  REDACTED
REDACTED
</script>
