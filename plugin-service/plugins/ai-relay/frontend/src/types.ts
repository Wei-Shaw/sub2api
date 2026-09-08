export interface RelayRoute {
  platform: string
  slug: string
  name: string
  base_url: string
  path_mappings: Record<string, string>
}

export interface Platform {
  key: string
  display_name: string
  default_base_url?: string
}

export interface RelayRuntime {
  base_url: string
}

export interface Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface MappingRow {
  id: number
  source: string
  target: string
}

export interface RoutePage {
  items: RelayRoute[]
  pagination: Pagination
}
