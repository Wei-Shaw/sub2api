<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex gap-2">
        <button
          v-for="item in tabs"
          :key="item"
          :class="['btn btn-sm', tab === item ? 'btn-primary' : 'btn-secondary']"
          @click="switchTab(item)"
        >
          {{ t(`imageStudio.tabs.${item}`) }}
        </button>
      </div>

      <div v-if="!loadingKeys && eligibleKeys.length === 0" class="card p-6 text-body text-fg-muted">
        {{ t('imageStudio.noEligibleKey') }}
        <router-link to="/keys" class="text-accent">{{ t('imageStudio.manageKeys') }}</router-link>
      </div>

      <template v-else-if="tab === 'create'">
        <section class="card space-y-4 p-4 sm:p-5">
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <label class="input-label">{{ t('imageStudio.apiKey') }}</label>
              <Select v-model="form.apiKeyId" :options="keyOptions" :disabled="submitting" />
            </div>
            <div>
              <label class="input-label">{{ t('imageStudio.model') }}</label>
              <Select v-model="form.model" :options="modelOptions" :disabled="submitting" creatable searchable />
            </div>
            <div>
              <label class="input-label">{{ t('imageStudio.size') }}</label>
              <Select v-model="form.size" :options="sizeOptions" :disabled="submitting" />
            </div>
            <div>
              <label class="input-label">{{ t('imageStudio.count') }}</label>
              <Select v-model="form.n" :options="countOptions" :disabled="submitting" />
            </div>
            <div>
              <label class="input-label">{{ t('imageStudio.quality') }}</label>
              <Select v-model="form.quality" :options="optionList(['', 'low', 'medium', 'high'])" :disabled="submitting" />
            </div>
            <div>
              <label class="input-label">{{ t('imageStudio.format') }}</label>
              <Select v-model="form.outputFormat" :options="optionList(['', 'png', 'jpeg', 'webp'])" :disabled="submitting" />
            </div>
            <div>
              <label class="input-label">{{ t('imageStudio.background') }}</label>
              <Select v-model="form.background" :options="optionList(['', 'transparent', 'opaque'])" :disabled="submitting" />
            </div>
          </div>

          <div>
            <label class="input-label" for="image-studio-prompt">{{ t('imageStudio.prompt') }}</label>
            <textarea id="image-studio-prompt" v-model="form.prompt" rows="4" maxlength="8000" class="input" :disabled="submitting" />
          </div>

          <div
            class="rounded-md border-2 border-dashed border-border p-3 text-meta text-fg-muted"
            @dragover.prevent
            @drop.prevent="addFiles($event.dataTransfer?.files)"
          >
            <div class="flex flex-wrap items-center gap-3">
              <label class="btn btn-secondary btn-sm cursor-pointer">
                <Icon name="upload" size="sm" class="mr-1" />
                {{ t('imageStudio.addReference') }}
                <input type="file" accept="image/png,image/jpeg,image/webp" multiple class="sr-only" @change="onFileInput" />
              </label>
              <span>{{ t('imageStudio.referenceHint', { max: IMAGE_STUDIO_MAX_INPUT_IMAGES }) }}</span>
            </div>
            <div v-if="inputImages.length" class="mt-3 flex flex-wrap gap-2">
              <div v-for="(image, index) in inputImages" :key="index" class="relative">
                <img :src="image" :alt="t('imageStudio.reference', { n: index + 1 })" class="h-20 w-20 rounded object-cover" />
                <button
                  class="absolute -right-2 -top-2 rounded-full bg-danger p-0.5 text-white"
                  :aria-label="t('common.delete')"
                  @click="inputImages.splice(index, 1)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </div>
            </div>
          </div>

          <div class="flex justify-end">
            <button class="btn btn-primary" :disabled="!canSubmit" @click="submit">
              <Icon name="sparkles" size="md" class="mr-2" :class="submitting ? 'animate-pulse' : ''" />
              {{ inputImages.length ? t('imageStudio.edit') : t('imageStudio.generate') }}
            </button>
          </div>
        </section>

        <section class="space-y-3">
          <div v-for="job in jobs" :key="job.id" class="card p-4">
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2 text-meta">
                  <span :class="['badge', statusBadge(job.status)]">{{ t(`imageStudio.status.${job.status}`) }}</span>
                  <span class="text-fg-subtle">#{{ job.id }} · {{ job.model }} · {{ formatTime(job.created_at) }}</span>
                  <span v-if="job.duration_ms" class="tabular-nums text-fg-subtle">{{ (job.duration_ms / 1000).toFixed(1) }}s</span>
                </div>
                <p class="mt-1 line-clamp-2 text-body text-fg">{{ job.prompt }}</p>
                <p v-if="job.error_message" class="mt-1 text-meta text-danger">{{ job.error_message }}</p>
                <p v-if="job.warning" class="mt-1 text-meta text-warning">{{ job.warning }}</p>
              </div>
              <div class="flex gap-1">
                <button class="btn btn-ghost btn-sm btn-icon" :title="t('imageStudio.reuse')" :aria-label="t('imageStudio.reuse')" @click="reuse(job)">
                  <Icon name="refresh" size="sm" />
                </button>
                <button class="btn btn-ghost btn-sm btn-icon" :title="t('imageStudio.copyPrompt')" :aria-label="t('imageStudio.copyPrompt')" @click="copyToClipboard(job.prompt)">
                  <Icon name="copy" size="sm" />
                </button>
                <button
                  class="btn btn-ghost btn-sm btn-icon"
                  :disabled="isUnfinished(job)"
                  :title="t('common.delete')"
                  :aria-label="t('common.delete')"
                  @click="removeJob(job)"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
            <div v-if="job.assets?.length" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
              <AssetTile v-for="asset in job.assets" :key="asset.id" :asset="asset" />
            </div>
          </div>
          <div v-if="jobsHasMore" class="flex justify-center">
            <button class="btn btn-secondary btn-sm" @click="loadJobs(jobsPage + 1)">{{ t('imageStudio.loadMore') }}</button>
          </div>
        </section>
      </template>

      <template v-else>
        <div v-if="assets.length === 0" class="card p-6 text-body text-fg-muted">{{ t('imageStudio.emptyGallery') }}</div>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          <AssetTile v-for="asset in assets" :key="asset.id" :asset="asset" deletable />
        </div>
        <div v-if="assetsHasMore" class="flex justify-center">
          <button class="btn btn-secondary btn-sm" @click="loadAssets(assetsPage + 1)">{{ t('imageStudio.loadMore') }}</button>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, reactive, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { keysAPI } from '@/api/keys'
import { saveBlob } from '@/api/batchImage'
import { imageStudioAPI, type ImageStudioAsset, type ImageStudioJob } from '@/api/imageStudio'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { ApiKey } from '@/types'
import {
  IMAGE_STUDIO_MAX_INPUT_IMAGES,
  defaultImageModel,
  readAsDataURL,
  validateInputImage
} from './imageStudioFiles'

type Tab = 'create' | 'gallery'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const tabs: Tab[] = ['create', 'gallery']
const tab = ref<Tab>('create')
const loadingKeys = ref(true)
const eligibleKeys = ref<ApiKey[]>([])
const submitting = ref(false)
const inputImages = ref<string[]>([])
const form = reactive({ apiKeyId: 0, model: '', size: '', n: 1, quality: '', outputFormat: '', background: '', prompt: '' })

const jobs = ref<ImageStudioJob[]>([])
const jobsPage = ref(1)
const jobsHasMore = ref(false)
const assets = ref<ImageStudioAsset[]>([])
const assetsPage = ref(1)
const assetsHasMore = ref(false)

const keyOptions = computed(() => eligibleKeys.value.map((key) => ({ value: key.id, label: `${key.name} · ${key.group?.name ?? ''}` })))
const selectedKey = computed(() => eligibleKeys.value.find((key) => key.id === form.apiKeyId))
const modelOptions = computed(() => {
  const model = defaultImageModel(selectedKey.value?.group?.platform)
  return optionList(form.model && form.model !== model ? [model, form.model] : [model])
})
const sizeOptions = computed(() => optionList(['', '1024x1024', '1536x1024', '1024x1536']))
const countOptions = [1, 2, 3, 4].map((n) => ({ value: n, label: String(n) }))
const canSubmit = computed(() => !submitting.value && form.apiKeyId > 0 && form.model.trim() !== '' && form.prompt.trim() !== '')

function optionList(values: string[]) {
  return values.map((value) => ({ value, label: value || t('imageStudio.auto') }))
}

function isUnfinished(job: ImageStudioJob) {
  return job.status === 'queued' || job.status === 'running'
}

function statusBadge(status: ImageStudioJob['status']) {
  return { queued: 'badge-gray', running: 'badge-warning', succeeded: 'badge-success', failed: 'badge-danger' }[status]
}

function formatTime(value: string) {
  return new Date(value).toLocaleString()
}

function showApiError(err: unknown) {
  appStore.showError(extractApiErrorMessage(err, t('common.unknownError')))
}

async function loadKeys() {
  loadingKeys.value = true
  try {
    const res = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    eligibleKeys.value = res.items.filter(
      (key) => (key.group?.platform === 'openai' || key.group?.platform === 'grok') && key.group?.allow_image_generation === true
    )
    if (eligibleKeys.value.length && !form.apiKeyId) {
      form.apiKeyId = eligibleKeys.value[0].id
      form.model = defaultImageModel(eligibleKeys.value[0].group?.platform)
    }
  } catch (err) {
    showApiError(err)
  } finally {
    loadingKeys.value = false
  }
}

async function loadJobs(page = 1) {
  try {
    const res = await imageStudioAPI.listJobs(page, 20)
    jobs.value = page === 1 ? res.items : [...jobs.value, ...res.items]
    jobsPage.value = page
    jobsHasMore.value = page < res.pages
  } catch (err) {
    showApiError(err)
  }
}

async function loadAssets(page = 1) {
  try {
    const res = await imageStudioAPI.listAssets(page, 24)
    assets.value = page === 1 ? res.items : [...assets.value, ...res.items]
    assetsPage.value = page
    assetsHasMore.value = page < res.pages
  } catch (err) {
    showApiError(err)
  }
}

function switchTab(next: Tab) {
  tab.value = next
  if (next === 'gallery') void loadAssets(1)
}

async function addFiles(files: FileList | null | undefined) {
  for (const file of Array.from(files ?? [])) {
    if (inputImages.value.length >= IMAGE_STUDIO_MAX_INPUT_IMAGES) break
    const error = validateInputImage(file)
    if (error) {
      appStore.showError(t(`imageStudio.referenceError.${error}`, { name: file.name }))
      continue
    }
    inputImages.value.push(await readAsDataURL(file))
  }
}

function onFileInput(event: Event) {
  const input = event.target as HTMLInputElement
  void addFiles(input.files)
  input.value = ''
}

async function submit() {
  submitting.value = true
  try {
    const job = await imageStudioAPI.createJob({
      api_key_id: form.apiKeyId,
      model: form.model.trim(),
      prompt: form.prompt.trim(),
      size: form.size || undefined,
      quality: form.quality || undefined,
      output_format: form.outputFormat || undefined,
      background: form.background || undefined,
      n: form.n,
      input_images: inputImages.value.length ? inputImages.value : undefined
    })
    jobs.value = [job, ...jobs.value]
    inputImages.value = []
    schedulePoll()
  } catch (err) {
    showApiError(err)
  } finally {
    submitting.value = false
  }
}

function reuse(job: ImageStudioJob) {
  if (eligibleKeys.value.some((key) => key.id === job.api_key_id)) form.apiKeyId = job.api_key_id
  Object.assign(form, {
    model: job.model,
    prompt: job.prompt,
    size: job.params.size ?? '',
    quality: job.params.quality ?? '',
    outputFormat: job.params.output_format ?? '',
    background: job.params.background ?? '',
    n: job.params.n ?? 1
  })
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

async function removeJob(job: ImageStudioJob) {
  try {
    await imageStudioAPI.deleteJob(job.id)
    jobs.value = jobs.value.filter((item) => item.id !== job.id)
  } catch (err) {
    showApiError(err)
  }
}

async function removeAsset(asset: ImageStudioAsset) {
  try {
    await imageStudioAPI.deleteAsset(asset.id)
    assets.value = assets.value.filter((item) => item.id !== asset.id)
    for (const job of jobs.value) job.assets = job.assets?.filter((item) => item.id !== asset.id)
  } catch (err) {
    showApiError(err)
  }
}

// Poll only while something is unfinished; results land in the job list.
// disposed 防止卸载后仍在进行中的请求返回时重新挂上定时器。
let pollTimer: number | undefined
let disposed = false
function schedulePoll() {
  window.clearTimeout(pollTimer)
  if (disposed || !jobs.value.some(isUnfinished)) return
  pollTimer = window.setTimeout(async () => {
    await loadJobs(1)
    schedulePoll()
  }, 2500)
}

// Object URLs for authenticated image blobs, revoked on unmount.
const objectURLs = reactive(new Map<number, string>())
async function assetURL(id: number) {
  if (!objectURLs.has(id)) {
    objectURLs.set(id, '')
    try {
      objectURLs.set(id, URL.createObjectURL(await imageStudioAPI.fetchAssetBlob(id)))
    } catch {
      objectURLs.delete(id)
    }
  }
}

async function download(asset: ImageStudioAsset) {
  try {
    saveBlob(await imageStudioAPI.fetchAssetBlob(asset.id), `image-${asset.id}.${asset.mime_type.split('/')[1] ?? 'png'}`)
  } catch (err) {
    showApiError(err)
  }
}

const AssetTile = defineComponent({
  props: {
    asset: { type: Object as PropType<ImageStudioAsset>, required: true },
    deletable: Boolean
  },
  setup(props) {
    void assetURL(props.asset.id)
    return () => {
      const src = objectURLs.get(props.asset.id)
      const alt = props.asset.revised_prompt || t('imageStudio.image', { id: props.asset.id })
      return h('div', { class: 'group relative overflow-hidden rounded-md border border-border bg-surface-muted' }, [
        src
          ? h('a', { href: src, target: '_blank', rel: 'noopener' }, [h('img', { src, alt, class: 'aspect-square w-full object-cover', loading: 'lazy' })])
          : h('div', { class: 'aspect-square w-full animate-pulse' }),
        h('div', { class: 'absolute right-1 top-1 flex gap-1 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100' }, [
          h('button', { class: 'btn btn-secondary btn-sm btn-icon', title: t('imageStudio.download'), 'aria-label': t('imageStudio.download'), onClick: () => download(props.asset) }, [
            h(Icon, { name: 'download', size: 'sm' })
          ]),
          props.deletable
            ? h('button', { class: 'btn btn-secondary btn-sm btn-icon', title: t('common.delete'), 'aria-label': t('common.delete'), onClick: () => removeAsset(props.asset) }, [
                h(Icon, { name: 'trash', size: 'sm' })
              ])
            : null
        ])
      ])
    }
  }
})

onMounted(async () => {
  await Promise.all([loadKeys(), loadJobs(1)])
  schedulePoll()
})

onBeforeUnmount(() => {
  disposed = true
  window.clearTimeout(pollTimer)
  objectURLs.forEach((url) => url && URL.revokeObjectURL(url))
})
</script>
