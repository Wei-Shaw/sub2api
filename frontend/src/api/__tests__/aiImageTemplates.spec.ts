import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  findTemplate,
  loadTemplates,
  persistTemplates,
  removeTemplate,
  upsertTemplate,
  MAX_TEMPLATES,
  MAX_TEMPLATE_NAME,
  TEMPLATE_STORAGE_KEY,
  type PromptTemplate,
} from '../aiImageTemplates'

function template(name: string, prompt = 'p'): PromptTemplate {
  return { id: `id-${name}`, name, prompt, createdAt: 1, updatedAt: 1 }
}

describe('提示词模板：读取', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('没存过任何模板时返回空数组', () => {
    expect(loadTemplates()).toEqual([])
  })

  it('存过就能原样读回来', () => {
    window.localStorage.setItem(
      TEMPLATE_STORAGE_KEY,
      JSON.stringify([{ id: 'a', name: '白底图', prompt: '换成白底', createdAt: 1, updatedAt: 2 }]),
    )

    const result = loadTemplates()
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({ id: 'a', name: '白底图', prompt: '换成白底' })
  })

  // 用户可能手改过 localStorage，或者旧版本留下过别的格式。
  // 这种情况下必须当作「没有模板」，绝不能让整页白屏。
  it('内容不是 JSON 时返回空数组而不是抛错', () => {
    window.localStorage.setItem(TEMPLATE_STORAGE_KEY, '这不是 JSON{{{')
    expect(loadTemplates()).toEqual([])
  })

  it('内容不是数组时返回空数组', () => {
    window.localStorage.setItem(TEMPLATE_STORAGE_KEY, JSON.stringify({ name: '不是数组' }))
    expect(loadTemplates()).toEqual([])
  })

  it('丢掉缺字段的坏条目，保留好条目', () => {
    window.localStorage.setItem(
      TEMPLATE_STORAGE_KEY,
      JSON.stringify([
        { id: 'good', name: '好的', prompt: '正文', createdAt: 1, updatedAt: 1 },
        { id: 'no-prompt', name: '缺正文' },
        { name: '空正文', prompt: '   ' },
        null,
      ]),
    )

    const result = loadTemplates()
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('好的')
  })

  it('坏条目缺 id 时补一个，不会让两条模板撞成同一个 key', () => {
    window.localStorage.setItem(
      TEMPLATE_STORAGE_KEY,
      JSON.stringify([
        { name: 'A', prompt: 'a' },
        { name: 'B', prompt: 'b' },
      ]),
    )

    const result = loadTemplates()
    expect(result).toHaveLength(2)
    expect(result[0].id).toBeTruthy()
    expect(result[1].id).toBeTruthy()
    expect(result[0].id).not.toBe(result[1].id)
  })

  it('读超过上限的历史数据时只保留上限条数', () => {
    const many = Array.from({ length: MAX_TEMPLATES + 10 }, (_, index) => ({
      id: `id-${index}`,
      name: `模板 ${index}`,
      prompt: '正文',
      createdAt: 1,
      updatedAt: 1,
    }))
    window.localStorage.setItem(TEMPLATE_STORAGE_KEY, JSON.stringify(many))

    expect(loadTemplates()).toHaveLength(MAX_TEMPLATES)
  })
})

describe('提示词模板：保存', () => {
  it('新模板放在最前面，用户刚存的马上能看到', () => {
    const result = upsertTemplate([template('旧的')], '新的', '正文')
    expect(result.map((item) => item.name)).toEqual(['新的', '旧的'])
  })

  // 用户想改已有模板的内容时，不该被迫先删掉再重建。
  it('同名视为覆盖，不会存出两条同名的', () => {
    const result = upsertTemplate([template('白底图', '旧正文')], '白底图', '新正文')
    expect(result).toHaveLength(1)
    expect(result[0].prompt).toBe('新正文')
  })

  it('覆盖时保留原来的 id，界面上的选中状态不会错位', () => {
    const result = upsertTemplate([template('白底图')], '白底图', '新正文')
    expect(result[0].id).toBe('id-白底图')
  })

  it('名字两边的空格会去掉', () => {
    const result = upsertTemplate([], '  白底图  ', '正文')
    expect(result[0].name).toBe('白底图')
  })

  it('名字太长会截断，免得把下拉框撑坏', () => {
    const result = upsertTemplate([], 'x'.repeat(200), '正文')
    expect(result[0].name).toHaveLength(MAX_TEMPLATE_NAME)
  })

  it('名字为空时不存', () => {
    expect(upsertTemplate([], '   ', '正文')).toEqual([])
  })

  it('提示词为空时不存', () => {
    expect(upsertTemplate([], '白底图', '   ')).toEqual([])
  })

  it('超过上限时丢掉最旧的', () => {
    const full = Array.from({ length: MAX_TEMPLATES }, (_, index) => template(`t${index}`))
    const result = upsertTemplate(full, '最新的', '正文')
    expect(result).toHaveLength(MAX_TEMPLATES)
    expect(result[0].name).toBe('最新的')
    expect(result.some((item) => item.name === `t${MAX_TEMPLATES - 1}`)).toBe(false)
  })
})

describe('提示词模板：删除与查找', () => {
  it('按 id 删除', () => {
    const result = removeTemplate([template('A'), template('B')], 'id-A')
    expect(result.map((item) => item.name)).toEqual(['B'])
  })

  it('删不存在的 id 时原样返回', () => {
    const list = [template('A')]
    expect(removeTemplate(list, '不存在')).toHaveLength(1)
  })

  it('按 id 查找', () => {
    expect(findTemplate([template('A'), template('B')], 'id-B')?.name).toBe('B')
  })

  it('找不到返回 undefined', () => {
    expect(findTemplate([template('A')], '不存在')).toBeUndefined()
  })
})

describe('提示词模板：写回浏览器', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('写进去再读出来内容一致', () => {
    const list = [template('白底图', '换成白底')]
    expect(persistTemplates(list)).toBe(true)
    expect(loadTemplates()[0]).toMatchObject({ name: '白底图', prompt: '换成白底' })
  })

  // 隐私模式或配额满时 setItem 会抛。模板存不上不该妨碍用户正常生图，
  // 所以这里只返回 false，让界面提示一句。
  it('存储不可用时返回 false 而不是抛错', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError')
    })

    expect(() => persistTemplates([template('A')])).not.toThrow()
    expect(persistTemplates([template('A')])).toBe(false)

    spy.mockRestore()
  })
})
