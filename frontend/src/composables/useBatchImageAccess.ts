import { computed, ref REDACTED from 'vue'
import { keysAPI REDACTED from '@/api/keys'
import { useAuthStore REDACTED from '@/stores/auth'
import type { ApiKey REDACTED from '@/types'

const loaded = ref(false)
const loading = ref(false)
const hasAllowedBatchImageKey = ref(false)
let pendingLoad: Promise<boolean> | null = null
const pageSize = 100

function keyAllowsBatchImage(key: ApiKey): boolean {
  return (
    key.status === 'active' &&
    key.group?.platform === 'gemini' &&
    key.group?.allow_batch_image_generation === true
  )
REDACTED

async function loadBatchImageAccess(force = false): Promise<boolean> {
  const authStore = useAuthStore()
  if (!authStore.isAuthenticated) {
    loaded.value = true
    hasAllowedBatchImageKey.value = false
    return false
  REDACTED

  if (loaded.value && !force) {
    return hasAllowedBatchImageKey.value
  REDACTED

  if (pendingLoad && !force) {
    return pendingLoad
  REDACTED

  loading.value = true
  pendingLoad = (async () => {
    let page = 1
    while (true) {
      const response = await keysAPI.list(page, pageSize, {
        status: 'active',
        sort_by: 'created_at',
        sort_order: 'desc'
      REDACTED)

      if ((response.items || []).some(keyAllowsBatchImage)) {
        hasAllowedBatchImageKey.value = true
        loaded.value = true
        return true
      REDACTED

      if (page >= response.pages || (response.items || []).length === 0) {
        hasAllowedBatchImageKey.value = false
        loaded.value = true
        return false
      REDACTED

      page += 1
    REDACTED
  REDACTED)()
    .catch(() => {
      hasAllowedBatchImageKey.value = false
      loaded.value = true
      return false
    REDACTED)
    .finally(() => {
      loading.value = false
      pendingLoad = null
    REDACTED)

  return pendingLoad
REDACTED

export function useBatchImageAccess() {
  const canUseBatchImage = computed(() => hasAllowedBatchImageKey.value)

  return {
    canUseBatchImage,
    batchImageAccessLoaded: computed(() => loaded.value),
    batchImageAccessLoading: computed(() => loading.value),
    refreshBatchImageAccess: loadBatchImageAccess,
  REDACTED
REDACTED
