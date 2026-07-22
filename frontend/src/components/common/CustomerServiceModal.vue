<template>
  <button
    v-if="cards.length"
    ref="triggerRef"
    type="button"
    class="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
    :aria-label="t('common.customerService')"
    aria-haspopup="dialog"
    @click="openModal"
  >
    <Icon name="chat" size="sm" />
    <span class="hidden md:inline">{{ t('common.customerService') }}</span>
  </button>

  <Teleport to="body">
    <Transition name="customer-service-modal">
      <div
        v-if="isOpen"
        class="fixed inset-0 z-[100] flex items-start justify-center overflow-y-auto bg-gray-950/60 p-4 pt-[8vh] backdrop-blur-sm"
        role="presentation"
        @click.self="closeModal"
      >
        <section
          class="w-full max-w-[860px] overflow-hidden rounded-lg border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
        >
          <header class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:px-6">
            <div class="flex items-center gap-3">
              <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-200">
                <Icon name="chat" size="md" />
              </span>
              <div>
                <h2 :id="titleId" class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('common.customerService') }}
                </h2>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ t('common.customerServiceDescription') }}
                </p>
              </div>
            </div>
            <button
              ref="closeRef"
              type="button"
              class="btn-ghost btn-icon"
              :aria-label="t('common.closeCustomerService')"
              @click="closeModal"
            >
              <Icon name="x" size="md" />
            </button>
          </header>

          <div class="grid grid-cols-1 gap-6 px-5 py-6 md:grid-cols-2 md:px-6 md:py-8">
            <article
              v-for="card in cards"
              :key="card.kind"
              class="flex min-w-0 flex-col items-center rounded-lg border border-gray-200 bg-gray-50 p-5 text-center dark:border-dark-700 dark:bg-dark-800/60"
            >
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ card.title }}</h3>
              <div class="mt-4 flex h-[min(360px,65vw)] w-full max-w-[320px] items-center justify-center overflow-hidden rounded-lg bg-white p-3 dark:bg-white">
                <img
                  v-if="card.qrCode && !failedImages.has(card.kind)"
                  :src="card.qrCode"
                  :alt="card.title"
                  class="h-full w-full object-contain"
                  @error="markImageFailed(card.kind)"
                />
                <Icon v-else name="chat" size="xl" class="text-gray-300" />
              </div>
              <a
                v-if="card.link"
                :href="card.link"
                target="_blank"
                rel="noopener noreferrer"
                class="mt-4 max-w-full break-all text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
              >
                {{ card.linkLabel }}
              </a>
            </article>
          </div>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const props = defineProps<{
  afterSalesQrCode?: string
  afterSalesLink?: string
  officialGroupQrCode?: string
  officialGroupLink?: string
}>()

type CardKind = 'after-sales' | 'official-group'

interface CustomerServiceCard {
  kind: CardKind
  title: string
  qrCode: string
  link: string
  linkLabel: string
}

const { t } = useI18n()
const isOpen = ref(false)
const triggerRef = ref<HTMLButtonElement | null>(null)
const closeRef = ref<HTMLButtonElement | null>(null)
const generatedAfterSalesQr = ref('')
const generatedOfficialGroupQr = ref('')
const failedImages = ref(new Set<CardKind>())
const titleId = `customer-service-title-${Math.random().toString(36).slice(2, 9)}`
let previousBodyOverflow = ''
let generationSequence = 0

const afterSalesLink = computed(() => sanitizeUrl(props.afterSalesLink || ''))
const officialGroupLink = computed(() => sanitizeUrl(props.officialGroupLink || ''))
const afterSalesQrCode = computed(() =>
  sanitizeUrl(props.afterSalesQrCode || '', { allowRelative: true, allowDataUrl: true }),
)
const officialGroupQrCode = computed(() =>
  sanitizeUrl(props.officialGroupQrCode || '', { allowRelative: true, allowDataUrl: true }),
)

const cards = computed<CustomerServiceCard[]>(() => {
  const result: CustomerServiceCard[] = []
  const afterSalesQr = afterSalesQrCode.value || generatedAfterSalesQr.value
  const groupQr = officialGroupQrCode.value || generatedOfficialGroupQr.value

  if (afterSalesQr || afterSalesLink.value) {
    result.push({
      kind: 'after-sales',
      title: t('common.afterSalesSupport'),
      qrCode: afterSalesQr,
      link: afterSalesLink.value,
      linkLabel: t('common.openSupportLink'),
    })
  }
  if (groupQr || officialGroupLink.value) {
    result.push({
      kind: 'official-group',
      title: t('common.officialGroup'),
      qrCode: groupQr,
      link: officialGroupLink.value,
      linkLabel: t('common.openGroupLink'),
    })
  }
  return result
})

watch(
  [afterSalesLink, officialGroupLink, afterSalesQrCode, officialGroupQrCode],
  async ([supportLink, groupLink, supportQr, groupQr]) => {
    const sequence = ++generationSequence
    failedImages.value = new Set()
    const [generatedSupport, generatedGroup] = await Promise.all([
      !supportQr && supportLink ? generateQrCode(supportLink) : Promise.resolve(''),
      !groupQr && groupLink ? generateQrCode(groupLink) : Promise.resolve(''),
    ])
    if (sequence !== generationSequence) return
    generatedAfterSalesQr.value = generatedSupport
    generatedOfficialGroupQr.value = generatedGroup
  },
  { immediate: true },
)

async function generateQrCode(value: string): Promise<string> {
  try {
    return await QRCode.toDataURL(value, {
      width: 640,
      margin: 2,
      errorCorrectionLevel: 'M',
      color: { dark: '#111111', light: '#ffffff' },
    })
  } catch {
    return ''
  }
}

async function openModal() {
  previousBodyOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  isOpen.value = true
  await nextTick()
  closeRef.value?.focus()
}

function closeModal() {
  if (!isOpen.value) return
  isOpen.value = false
  document.body.style.overflow = previousBodyOverflow
  nextTick(() => triggerRef.value?.focus())
}

function markImageFailed(kind: CardKind) {
  failedImages.value = new Set(failedImages.value).add(kind)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeModal()
}

onMounted(() => document.addEventListener('keydown', onKeydown))

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  if (isOpen.value) document.body.style.overflow = previousBodyOverflow
})
</script>

<style scoped>
.customer-service-modal-enter-active,
.customer-service-modal-leave-active {
  transition: opacity 160ms ease;
}

.customer-service-modal-enter-active section,
.customer-service-modal-leave-active section {
  transition: transform 160ms ease, opacity 160ms ease;
}

.customer-service-modal-enter-from,
.customer-service-modal-leave-to {
  opacity: 0;
}

.customer-service-modal-enter-from section,
.customer-service-modal-leave-to section {
  opacity: 0;
  transform: translateY(-8px) scale(0.98);
}
</style>
