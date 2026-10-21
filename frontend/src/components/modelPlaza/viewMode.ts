export type ModelPlazaViewMode = 'list' | 'card'

export function resolveModelPlazaViewMode(saved: string | null): ModelPlazaViewMode {
  return saved === 'list' ? 'list' : 'card'
}
