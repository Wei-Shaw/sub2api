<template>
  <AppLayout>
    <div class="mx-auto max-w-[950px] space-y-6">
      <ProfileCheckInCard show-disabled @checked-in="loadCalendar" />

      <section class="card border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p class="text-sm font-medium text-gray-500 dark:text-gray-400">签到月历</p>
            <h2 class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">
              {{ monthTitle }}
            </h2>
          </div>

          <div class="flex items-center gap-2">
            <button
              type="button"
              class="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-gray-200 text-gray-600 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              title="上个月"
              aria-label="上个月"
              @click="goToPreviousMonth"
            >
              <Icon name="chevronLeft" size="sm" />
            </button>
            <button
              type="button"
              class="inline-flex h-9 min-w-[72px] items-center justify-center rounded-lg border border-gray-200 px-3 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
              @click="goToCurrentMonth"
            >
              本月
            </button>
            <button
              type="button"
              class="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-gray-200 text-gray-600 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              title="下个月"
              aria-label="下个月"
              @click="goToNextMonth"
            >
              <Icon name="chevronRight" size="sm" />
            </button>
          </div>
        </div>

        <div class="mt-5 grid gap-3 sm:grid-cols-3">
          <div class="rounded-lg bg-emerald-50 px-4 py-3 dark:bg-emerald-900/20">
            <p class="text-xs font-medium text-emerald-700 dark:text-emerald-300">本月已签到</p>
            <p class="mt-1 text-2xl font-semibold text-emerald-700 dark:text-emerald-200">
              {{ calendar?.checked_in_days ?? 0 }} 天
            </p>
          </div>
          <div class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-700/70">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">月份天数</p>
            <p class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ daysInSelectedMonth }} 天
            </p>
          </div>
          <div class="rounded-lg bg-amber-50 px-4 py-3 dark:bg-amber-900/20">
            <p class="text-xs font-medium text-amber-700 dark:text-amber-300">签到状态</p>
            <p class="mt-2 text-sm font-semibold text-amber-700 dark:text-amber-200">
              {{ statusLabel }}
            </p>
          </div>
        </div>

        <div
          v-if="loadError"
          class="mt-5 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
        >
          {{ loadError }}
        </div>

        <div class="mt-5 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <div class="grid grid-cols-7 bg-gray-50 dark:bg-dark-900/60">
            <div
              v-for="weekday in weekdays"
              :key="weekday"
              class="py-2 text-center text-xs font-semibold text-gray-500 dark:text-gray-400"
            >
              {{ weekday }}
            </div>
          </div>

          <div class="grid grid-cols-7 bg-white dark:bg-dark-800">
            <div
              v-for="day in calendarCells"
              :key="day.key"
              :class="[
                'relative flex aspect-square min-h-[54px] items-center justify-center border-t border-gray-100 text-sm dark:border-dark-700 sm:min-h-[76px]',
                day.inMonth
                  ? 'text-gray-700 dark:text-gray-200'
                  : 'bg-gray-50 text-gray-300 dark:bg-dark-900/40 dark:text-gray-600',
              ]"
            >
              <span
                v-if="day.inMonth"
                :class="[
                  'inline-flex h-9 w-9 items-center justify-center rounded-full font-medium',
                  day.checked
                    ? 'bg-emerald-500 text-white shadow-sm'
                    : day.today
                      ? 'ring-2 ring-amber-400 text-gray-900 dark:text-white'
                      : '',
                ]"
              >
                {{ day.day }}
              </span>
              <span v-else>{{ day.day }}</span>

              <span
                v-if="day.checked"
                class="absolute bottom-2 hidden text-[11px] font-medium text-emerald-600 dark:text-emerald-300 sm:block"
              >
                已签到
              </span>
            </div>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap items-center gap-4 text-xs text-gray-500 dark:text-gray-400">
          <span class="inline-flex items-center gap-2">
            <span class="h-3 w-3 rounded-full bg-emerald-500"></span>
            已签到
          </span>
          <span class="inline-flex items-center gap-2">
            <span class="h-3 w-3 rounded-full ring-2 ring-amber-400"></span>
            今天
          </span>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ProfileCheckInCard from '@/components/user/profile/ProfileCheckInCard.vue'
import { userAPI, type DailyCheckInCalendar } from '@/api/user'
import { useAppStore } from '@/stores/app'

interface CalendarCell {
  key: string
  day: number
  inMonth: boolean
  checked: boolean
  today: boolean
}

const appStore = useAppStore()
const weekdays = ['一', '二', '三', '四', '五', '六', '日']
const calendar = ref<DailyCheckInCalendar | null>(null)
const loading = ref(false)
const loadError = ref('')
const today = new Date()
const selectedYear = ref(today.getFullYear())
const selectedMonth = ref(today.getMonth() + 1)

const checkedDateSet = computed(() => new Set(calendar.value?.checked_in_dates ?? []))
const daysInSelectedMonth = computed(() => new Date(selectedYear.value, selectedMonth.value, 0).getDate())
const monthTitle = computed(() => `${selectedYear.value} 年 ${selectedMonth.value} 月`)
const statusLabel = computed(() => {
  if (loading.value) {
    return '加载中...'
  }
  if (calendar.value && !calendar.value.enabled) {
    return '签到功能未开启'
  }
  return (calendar.value?.checked_in_days ?? 0) > 0 ? '本月已有签到记录' : '本月暂无签到记录'
})

const calendarCells = computed<CalendarCell[]>(() => {
  const firstDate = new Date(selectedYear.value, selectedMonth.value - 1, 1)
  const leadingDays = (firstDate.getDay() + 6) % 7
  const daysInMonth = daysInSelectedMonth.value
  const previousMonthDays = new Date(selectedYear.value, selectedMonth.value - 1, 0).getDate()
  const cells: CalendarCell[] = []

  for (let index = leadingDays; index > 0; index--) {
    const date = new Date(selectedYear.value, selectedMonth.value - 2, previousMonthDays - index + 1)
    cells.push(buildCalendarCell(date, false))
  }

  for (let day = 1; day <= daysInMonth; day++) {
    const date = new Date(selectedYear.value, selectedMonth.value - 1, day)
    cells.push(buildCalendarCell(date, true))
  }

  while (cells.length % 7 !== 0) {
    const day = cells.length - leadingDays - daysInMonth + 1
    const date = new Date(selectedYear.value, selectedMonth.value, day)
    cells.push(buildCalendarCell(date, false))
  }

  return cells
})

function buildCalendarCell(date: Date, inMonth: boolean): CalendarCell {
  const dateKey = formatDateKey(date)
  return {
    key: `${dateKey}-${inMonth ? 'current' : 'adjacent'}`,
    day: date.getDate(),
    inMonth,
    checked: inMonth && checkedDateSet.value.has(dateKey),
    today: inMonth && dateKey === formatDateKey(today),
  }
}

function formatDateKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function goToPreviousMonth() {
  const date = new Date(selectedYear.value, selectedMonth.value - 2, 1)
  selectedYear.value = date.getFullYear()
  selectedMonth.value = date.getMonth() + 1
}

function goToNextMonth() {
  const date = new Date(selectedYear.value, selectedMonth.value, 1)
  selectedYear.value = date.getFullYear()
  selectedMonth.value = date.getMonth() + 1
}

function goToCurrentMonth() {
  selectedYear.value = today.getFullYear()
  selectedMonth.value = today.getMonth() + 1
}

async function loadCalendar() {
  loading.value = true
  loadError.value = ''
  try {
    calendar.value = await userAPI.getDailyCheckInCalendar(selectedYear.value, selectedMonth.value)
  } catch (error: any) {
    loadError.value = error?.message || '签到月历加载失败，请稍后重试'
    appStore.showError(loadError.value)
  } finally {
    loading.value = false
  }
}

watch([selectedYear, selectedMonth], () => {
  loadCalendar().catch((error) => {
    console.error('Failed to reload daily check-in calendar:', error)
  })
})

onMounted(() => {
  loadCalendar().catch((error) => {
    console.error('Failed to load daily check-in calendar:', error)
  })
})
</script>
