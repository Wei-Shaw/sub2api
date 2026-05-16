declare module '@tanstack/vue-virtual' {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  export function useVirtualizer(options: any): any
}

interface Window {
  __APP_CONFIG__?: Record<string, unknown>
}
