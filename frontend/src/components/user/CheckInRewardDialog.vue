<template>
  <BaseDialog
    :show="show"
    :title="t('dashboard.checkInSuccessTitle')"
    width="narrow"
    @close="emit('close')"
  >
    <div class="py-4 text-center">
      <span class="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-lime-200 text-gray-950 dark:bg-lime-300">
        <Icon name="gift" size="lg" />
      </span>
      <template v-if="holiday">
        <p class="mt-5 text-2xl font-semibold text-amber-600 dark:text-amber-400">
          {{ t(`checkIn.${holiday}Title`) }}
        </p>
        <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">
          {{ t(blessing) }}
        </p>
      </template>
      <p class="mt-5 text-4xl font-semibold tracking-normal text-gray-950 dark:text-white">
        +${{ Number(record?.reward || 0).toFixed(2) }}
      </p>
      <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">
        {{ t('dashboard.checkInSuccessDesc') }}
      </p>
    </div>
    <template #footer>
      <button type="button" class="btn btn-primary w-full sm:w-auto" @click="emit('close')">
        {{ t('common.confirm') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CheckInRecord } from '@/api/checkin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; record: CheckInRecord | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t } = useI18n()
const blessing = ref('')
const blessings = {
  midAutumn: [
    'checkIn.midAutumnBlessing',
    'checkIn.midAutumnBlessing2',
    'checkIn.midAutumnBlessing3',
    'checkIn.midAutumnBlessing4',
    'checkIn.midAutumnBlessing5'
  ],
  nationalDay: [
    'checkIn.nationalDayBlessing',
    'checkIn.nationalDayBlessing2',
    'checkIn.nationalDayBlessing3',
    'checkIn.nationalDayBlessing4',
    'checkIn.nationalDayBlessing5'
  ]
}

const holiday = computed(() => {
  // 跟随 2026 年双节活动，使用后端签到日期，避免客户端时区影响。
  const date = props.record?.date || ''
  if (date >= '2026-09-25' && date <= '2026-09-30') return 'midAutumn'
  if (date >= '2026-10-01' && date <= '2026-10-07') return 'nationalDay'
  return null
})

watch(
  [() => props.show, () => props.record?.id, holiday],
  ([show, , festival]) => {
    if (!show || !festival) return
    const choices = blessings[festival]
    // 弹窗打开时选定，余额刷新、重渲染或切换语言时保持同一条祝福。
    blessing.value = choices[Math.floor(Math.random() * choices.length)]
  },
  { immediate: true }
)
</script>
