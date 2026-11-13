import { createSharedComposable, useNow } from '@vueuse/core'

// All quota windows share a clock; ticking never requests usage data.
export const useQuotaNow = createSharedComposable(() => useNow({ interval: 30_000 }))
