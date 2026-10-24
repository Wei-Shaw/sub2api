import { defineComponent, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import SelectComponent from '@/components/common/Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

// Transition 离场在 jsdom 里要等一帧才移除节点。
const settle = () => new Promise((resolve) => setTimeout(resolve, 50))

afterEach(() => {
  document.body.innerHTML = ''
})

// 回归：BaseDialog 内容层带 @click.stop，冒泡阶段的 document click 监听收不到，
// 弹窗内的下拉浮层永远关不掉。这里用 @click.stop 容器模拟弹窗内容层。
const Host = defineComponent({
  components: { SelectComp: SelectComponent },
  setup() {
    const value = ref<number | null>(null)
    const options = [
      { value: 1, label: 'one' },
      { value: 2, label: 'two' }
    ]
    return { value, options }
  },
  template: `
    <div class="fake-dialog" @click.stop>
      <SelectComp
        v-model="value"
        :options="options"
        :searchable="true"
        data-testid="subject"
      />
      <button type="button" data-testid="inside-other">inside</button>
    </div>
  `
})

const dropdown = () => document.body.querySelector('.select-dropdown-portal')

describe('Select（弹窗内下拉收起）', () => {
  it('@click.stop 容器内点击其他位置也收起下拉（捕获阶段监听）', async () => {
    const wrapper = mount(Host, { attachTo: document.body })
    await wrapper.get('[data-testid="subject"] button').trigger('click')
    expect(dropdown()).not.toBeNull()

    await wrapper.get('[data-testid="inside-other"]').trigger('click')
    await settle()
    expect(dropdown()).toBeNull()

    wrapper.unmount()
  })

  it('Escape 收起下拉', async () => {
    const wrapper = mount(Host, { attachTo: document.body })
    await wrapper.get('[data-testid="subject"] button').trigger('click')
    expect(dropdown()).not.toBeNull()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    await settle()
    expect(dropdown()).toBeNull()

    wrapper.unmount()
  })

  it('点击选项仍能选中且不被 click-outside 误关', async () => {
    const wrapper = mount(Host, { attachTo: document.body })
    await wrapper.get('[data-testid="subject"] button').trigger('click')

    const options = document.body.querySelectorAll('.select-option') as NodeListOf<HTMLElement>
    expect(options.length).toBe(2)
    options[1].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    await settle()

    expect(dropdown()).toBeNull()
    expect((wrapper.vm as unknown as { value: number | null }).value).toBe(2)

    wrapper.unmount()
  })
})
