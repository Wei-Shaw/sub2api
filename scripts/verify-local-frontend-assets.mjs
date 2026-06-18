#!/usr/bin/env node

const baseUrl = (process.env.BASE_URL || 'http://127.0.0.1:18080').replace(/\/+$/, '')

const assetPattern = /(?:href|src)="(\/assets\/[^"]+\.(?:js|css))"/g
const jsImportPattern = /(?:import\(|from\s*)["']\.?\/?([^"']+\.js)["']/g

const unique = (values) => [...new Set(values)]

const assert = (condition, message) => {
  if (!condition) throw new Error(message)
}

const requestText = async (path) => {
  const response = await fetch(`${baseUrl}${path}`)
  const text = await response.text()
  return { response, text }
}

const collectHtmlAssets = (html) => unique([...html.matchAll(assetPattern)].map((match) => match[1]))

const normalizeAssetPath = (assetPath, importPath) => {
  if (importPath.startsWith('/')) return importPath
  const prefix = assetPath.slice(0, assetPath.lastIndexOf('/') + 1)
  return `${prefix}${importPath.replace(/^\.\//, '')}`
}

const collectImportedJsAssets = (assetPath, js) =>
  unique([...js.matchAll(jsImportPattern)].map((match) => normalizeAssetPath(assetPath, match[1])))
    .filter((path) => path.startsWith('/assets/'))

const verifyAsset = async (assetPath, seen) => {
  if (seen.has(assetPath)) return []
  seen.add(assetPath)

  const { response, text } = await requestText(assetPath)
  assert(response.ok, `${assetPath} HTTP ${response.status}`)
  assert(!assetPath.includes('/vendor-'), `${assetPath} 使用了 vendor-* 文件名`)
  assert(!text.includes('vendor-'), `${assetPath} 内容仍包含 vendor-`)

  if (!assetPath.endsWith('.js')) return []
  return collectImportedJsAssets(assetPath, text)
}

const run = async () => {
  const { response, text: html } = await requestText('/')
  assert(response.ok, `/ HTTP ${response.status}`)
  assert(!html.includes('vendor-'), '根 HTML 仍包含 vendor-')

  const queue = collectHtmlAssets(html)
  assert(queue.length > 0, '根 HTML 未发现任何静态资源')

  const seen = new Set()
  for (let index = 0; index < queue.length; index += 1) {
    const importedAssets = await verifyAsset(queue[index], seen)
    for (const asset of importedAssets) {
      if (!seen.has(asset)) queue.push(asset)
    }
  }

  console.log(JSON.stringify({
    baseUrl,
    htmlAssets: collectHtmlAssets(html).length,
    checkedAssets: seen.size,
    vendorReferences: 0
  }, null, 2))
}

run().catch((error) => {
  console.error(error.message)
  process.exit(1)
})
