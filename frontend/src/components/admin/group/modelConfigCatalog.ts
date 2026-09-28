import type { ModelConfigSearchResult } from '@/api/admin/groups'

// 目录加载后预先生成搜索文本，逐次输入不重复处理完整元数据。
export function indexModelConfigCatalog(models: ModelConfigSearchResult[]) {
  return models.map(model => ({
    model,
    id: model.id.toLowerCase(),
    text: `${model.provider} ${model.provider_name ?? ''} ${model.id} ${model.name}`.toLowerCase(),
  })).sort((left, right) => {
    const a = `${left.model.provider}/${left.model.id}`
    const b = `${right.model.provider}/${right.model.id}`
    return a < b ? -1 : a > b ? 1 : 0
  })
}

export function searchModelConfigCatalog(index: ReturnType<typeof indexModelConfigCatalog>, query: string) {
  const keyword = query.trim().toLowerCase()
  if (!keyword) return []
  const matches = index.filter(entry => entry.text.includes(keyword))
  // 精确 ID 优先，其余保留供应商/ID 的稳定顺序。只限制渲染量，不截断下载目录。
  return [...matches.filter(entry => entry.id === keyword), ...matches.filter(entry => entry.id !== keyword)]
    .slice(0, 100).map(entry => entry.model)
}
