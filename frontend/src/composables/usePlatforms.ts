import { ref, computed } from 'vue'
import { listPlatforms, type PlatformDeclaration } from '@/api/admin/platforms'

const platforms = ref<PlatformDeclaration[]>([])
const loaded = ref(false)
const loading = ref(false)
let inflightPromise: Promise<void> | null = null

export function usePlatforms() {
  async function fetchPlatforms() {
    if (loaded.value) return
    if (inflightPromise) return inflightPromise
    inflightPromise = doFetch()
    return inflightPromise
  }

  async function doFetch() {
    loading.value = true
    try {
      platforms.value = await listPlatforms()
      loaded.value = true
    } catch {
      // Platforms API may not be available if plugins are disabled
    } finally {
      loading.value = false
      inflightPromise = null
    }
  }

  async function refreshPlatforms() {
    loaded.value = false
    inflightPromise = null
    await fetchPlatforms()
  }

  const platformOptions = computed(() =>
    platforms.value.map(p => ({
      value: p.platform,
      label: p.display_name,
    }))
  )

  const typeOptionsForPlatform = computed(() => (platform: string) => {
    const p = platforms.value.find(d => d.platform === platform)
    if (!p) return []
    return p.account_types.map(at => ({
      value: at.type,
      label: at.display_name,
      description: at.description,
    }))
  })

  function getPlatformDecl(platform: string): PlatformDeclaration | undefined {
    return platforms.value.find(p => p.platform === platform)
  }

  function getAccountTypeDecl(platform: string, type: string) {
    const p = getPlatformDecl(platform)
    if (!p) return undefined
    return p.account_types.find(at => at.type === type)
  }

  return {
    platforms,
    loaded,
    loading,
    fetchPlatforms,
    refreshPlatforms,
    platformOptions,
    typeOptionsForPlatform,
    getPlatformDecl,
    getAccountTypeDecl,
  }
}
