import { apiClient } from '../client'

export interface ThemeConfigOption {
  label: string
  value: string
}

export interface ThemeConfigItem {
  key: string
  label: string
  type: 'title' | 'color' | 'text' | 'select' | 'number' | 'boolean'
  default: string
  options?: ThemeConfigOption[]
  description?: string
}

export interface ThemeMetadata {
  name: string
  short: string
  version: string
  author: string
  description: string
  preview: string
  main_css: string
  config?: ThemeConfigItem[]
}

export interface Theme {
  metadata: ThemeMetadata
  installed_at: string
  is_active: boolean
  config?: Record<string, string>
}

export interface ThemeListResponse {
  themes: Theme[]
}

export interface ThemeActiveResponse {
  active: boolean
  theme?: Theme
}

export const themesAPI = {
  list: () => apiClient.get<ThemeListResponse>('/admin/themes'),

  get: (short: string) => apiClient.get<Theme>(`/admin/themes/${short}`),

  getActive: () => apiClient.get<ThemeActiveResponse>('/admin/themes/active'),

  install: (file: File) => {
    const formData = new FormData()
    formData.append('file', file)
    return apiClient.post<Theme>('/admin/themes/install', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 120000,
    })
  },

  installFromGitHub: (url: string) =>
    apiClient.post<Theme>('/admin/themes/install-github', { url }),

  activate: (short: string) =>
    apiClient.post(`/admin/themes/${short}/activate`),

  deactivate: () => apiClient.post('/admin/themes/deactivate'),

  delete: (short: string) =>
    apiClient.delete(`/admin/themes/${short}`),

  updateConfig: (short: string, config: Record<string, string>) =>
    apiClient.put(`/admin/themes/${short}/config`, config),
}

export default themesAPI
