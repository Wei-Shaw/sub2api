import { apiClient } from '../client'
import type { ShopPage, ShopProduct } from '../shop'

export interface ShopOverview {
  product_count: number
  active_products: number
  available_stock: number
  sold_count: number
  revenue: number
}

export interface ShopInventoryCode {
  id: number
  code: string
  status: 'available' | 'sold' | 'disabled'
  buyer: string
  sold_at?: string
  created_at: string
}

export interface ShopAdminOrder {
  id: number
  order_no: string
  product_id: number
  product_name: string
  price: number
  buyer: string
  code: string
  status: string
  created_at: string
}

export const adminShopAPI = {
  async getOverview() {
    const { data } = await apiClient.get<ShopOverview>('/admin/shop/overview')
    return data
  },
  async listProducts() {
    const { data } = await apiClient.get<ShopProduct[]>('/admin/shop/products')
    return data
  },
  async createProduct(payload: { name: string; description: string; price: number; limit_per_user?: number | null }) {
    const { data } = await apiClient.post<ShopProduct>('/admin/shop/products', payload)
    return data
  },
  async updateProduct(id: number, payload: { name: string; description: string; price: number; status: string; limit_per_user?: number | null }) {
    const { data } = await apiClient.put<ShopProduct>(`/admin/shop/products/${id}`, payload)
    return data
  },
  async addCodes(id: number, codes: string) {
    const { data } = await apiClient.post<{ added: number }>(`/admin/shop/products/${id}/codes`, { codes })
    return data
  },
  async deleteProduct(id: number) { await apiClient.delete(`/admin/shop/products/${id}`) },
  async listInventory(id: number, params: { page: number; page_size: number; status?: string }) {
    const { data } = await apiClient.get<ShopPage<ShopInventoryCode>>(`/admin/shop/products/${id}/inventory`, { params })
    return data
  },
  async listOrders(id: number, params: { page: number; page_size: number; search?: string }) {
    const { data } = await apiClient.get<ShopPage<ShopAdminOrder>>(`/admin/shop/products/${id}/orders`, { params })
    return data
  },
  async updateInventoryStatus(productId: number, inventoryId: number, status: 'available' | 'disabled') { await apiClient.patch(`/admin/shop/products/${productId}/inventory/${inventoryId}`, { status }) },
  async deleteInventory(productId: number, ids: number[]) { const { data } = await apiClient.delete<{ deleted: number }>(`/admin/shop/products/${productId}/inventory`, { data: { ids } }); return data },
}

export default adminShopAPI
