import { apiClient } from './client'

export interface ShopProduct {
  id: number
  name: string
  description: string
  price: number
  status: string
  available_stock: number
  sold_count: number
  limit_per_user?: number | null
  created_at: string
  updated_at: string
}

export interface ShopPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface ShopOrder {
  id: number
  order_no: string
  product_id: number
  product_name: string
  price: number
  code: string
  status: string
  created_at: string
}

export const shopAPI = {
  async listProducts() {
    const { data } = await apiClient.get<ShopProduct[]>('/shop/products')
    return data
  },
  async purchase(productId: number, idempotencyKey = crypto.randomUUID()) {
    const { data } = await apiClient.post<ShopOrder>('/shop/orders', { product_id: productId }, {
      headers: { 'Idempotency-Key': idempotencyKey },
    })
    return data
  },
  async listOrders() {
    const { data } = await apiClient.get<ShopOrder[]>('/shop/orders')
    return data
  },
}

export default shopAPI
