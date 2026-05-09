export interface ModelCatalogPricing {
  cache_read?: number
  cache_write?: number
  input?: number
  output?: number
}

export interface ModelCatalogItem {
  model_id: string
  model_name: string
  developer_id?: number
  desc?: string
  pricing?: ModelCatalogPricing
  types?: string
  features?: string
  input_modalities?: string
  endpoints?: string
  max_output?: number
  context_length?: number
}

interface ModelCatalogResponse {
  data?: ModelCatalogItem[]
  message?: string
  success?: boolean
}

const MODEL_CATALOG_URL = 'https://aihubmix.com/api/v1/models'

export const modelCatalogAPI = {
  async list(): Promise<ModelCatalogItem[]> {
    const response = await fetch(MODEL_CATALOG_URL, {
      method: 'GET',
      headers: {
        Accept: 'application/json'
      },
      credentials: 'omit'
    })

    if (!response.ok) {
      throw new Error(`Failed to load model catalog: ${response.status}`)
    }

    const payload = (await response.json()) as ModelCatalogResponse
    if (!Array.isArray(payload.data)) {
      throw new Error(payload.message || 'Invalid model catalog response')
    }

    return payload.data
  }
}

export default modelCatalogAPI
