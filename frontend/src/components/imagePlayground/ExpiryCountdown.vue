<template>
  <span
    class="inline-flex max-w-full items-center gap-1 rounded-md bg-black/70 px-2 py-1 text-[11px] font-medium tabular-nums text-white shadow-sm backdrop-blur"
    :class="expired ? 'bg-red-700/85' : ''"
    :title="title"
  >
    <Icon name="clock" size="xs" class="flex-none" />
    <span class="truncate">{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  expiresAt: number
  now: number
}>()

const { t, locale } = useI18n()

const expiryMillis = computed(() => props.expiresAt < 10_000_000_000
  ? props.expiresAt * 1000
  : props.expiresAt)
const remainingSeconds = computed(() => Math.max(0, Math.ceil((expiryMillis.value - props.now) / 1000)))
const expired = computed(() => expiryMillis.value <= props.now)

const label = computed(() => {
  if (expired.value) return t('imagePlayground.retention.expired')
  let seconds = remainingSeconds.value
  const days = Math.floor(seconds / 86400)
  seconds %= 86400
  const hours = Math.floor(seconds / 3600)
  seconds %= 3600
  const minutes = Math.floor(seconds / 60)
  seconds %= 60
  const clock = [hours, minutes, seconds].map(value => String(value).padStart(2, '0')).join(':')
  const duration = days > 0 ? `${days}${t('imagePlayground.retention.dayUnit')} ${clock}` : clock
  return t('imagePlayground.retention.countdown', { time: duration })
})

const title = computed(() => t('imagePlayground.retention.expiresAt', {
  time: new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(expiryMillis.value)),
}))
</script>
