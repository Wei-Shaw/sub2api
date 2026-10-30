import { describe, expect, it } from 'vitest'

import source from '../GroupsView.vue?raw'

describe('MiniMax group permission wiring', () => {
  it.each(['createForm', 'editForm'])('binds %s media permission to the persisted group flag', (form) => {
    const blocks = source.match(/<MiniMaxGroupPricing\b[\s\S]*?\/>/g) ?? []
    const block = blocks.find((item) => item.includes(`${form}.minimax_media_prices`))
    expect(block).toContain(`v-model:allow-generation="${form}.allow_image_generation"`)
    expect(block).toContain(`:show-generation-permission="${form}.platform === 'minimax'"`)
  })
})
