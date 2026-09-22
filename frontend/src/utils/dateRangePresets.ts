import { formatDateLocalInput } from '@/utils/format'

export interface DatePreset {
  labelKey: string
  value: string
  getRange: () => { start: string; end: string }
}

export const dateRangePresets: DatePreset[] = [
  {
    labelKey: 'dates.today',
    value: 'today',
    getRange: () => {
      const today = formatDateLocalInput(new Date())
      return { start: today, end: today }
    }
  },
  {
    labelKey: 'dates.yesterday',
    value: 'yesterday',
    getRange: () => {
      const date = new Date()
      date.setDate(date.getDate() - 1)
      const yesterday = formatDateLocalInput(date)
      return { start: yesterday, end: yesterday }
    }
  },
  {
    labelKey: 'dates.last24Hours',
    value: 'last24Hours',
    getRange: () => {
      const end = new Date()
      const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
      return {
        start: formatDateLocalInput(start),
        end: formatDateLocalInput(end)
      }
    }
  },
  {
    labelKey: 'dates.last7Days',
    value: '7days',
    getRange: () => {
      const end = formatDateLocalInput(new Date())
      const date = new Date()
      date.setDate(date.getDate() - 6)
      const start = formatDateLocalInput(date)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last14Days',
    value: '14days',
    getRange: () => {
      const end = formatDateLocalInput(new Date())
      const date = new Date()
      date.setDate(date.getDate() - 13)
      const start = formatDateLocalInput(date)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last30Days',
    value: '30days',
    getRange: () => {
      const end = formatDateLocalInput(new Date())
      const date = new Date()
      date.setDate(date.getDate() - 29)
      const start = formatDateLocalInput(date)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.thisMonth',
    value: 'thisMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateLocalInput(new Date(now.getFullYear(), now.getMonth(), 1))
      return { start, end: formatDateLocalInput(new Date()) }
    }
  },
  {
    labelKey: 'dates.lastMonth',
    value: 'lastMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateLocalInput(new Date(now.getFullYear(), now.getMonth() - 1, 1))
      const end = formatDateLocalInput(new Date(now.getFullYear(), now.getMonth(), 0))
      return { start, end }
    }
  }
]
