import { mount, RouterLinkStub } from '@vue/test-utils'
import { expect, it } from 'vitest'
import SidebarNavGroup from '../SidebarNavGroup.vue'

const item = {
  label: '钉钉组织管理', icon: 'span', children: [
    { path: '/organization/dingtalk', label: '钉钉组织', icon: 'span' },
    { path: '/organization/quota', label: '组织额度分配', icon: 'span' },
    { path: '/organization/statistics', label: '组织部门额度统计', icon: 'span' }
  ]
}
it('shows the organization links only when expanded and emits navigation', async () => {
  const wrapper = mount(SidebarNavGroup, { props: { item, collapsed: false, expanded: false, active: true, activePath: '/organization/quota' }, global: { stubs: { RouterLink: RouterLinkStub } } })
  expect(wrapper.findAllComponents(RouterLinkStub)).toHaveLength(0)
  await wrapper.get('button').trigger('click')
  expect(wrapper.emitted('toggle')).toHaveLength(1)
  await wrapper.setProps({ expanded: true })
  expect(wrapper.get('button').attributes('aria-expanded')).toBe('true')
  const links = wrapper.findAllComponents(RouterLinkStub)
  expect(links).toHaveLength(3)
  expect(links[1]!.attributes('aria-current')).toBe('page')
  await links[1]!.trigger('click')
  expect(wrapper.emitted('navigate')).toEqual([['/organization/quota']])
  await wrapper.setProps({ collapsed: true })
  expect(wrapper.findAllComponents(RouterLinkStub)).toHaveLength(0)
  expect(wrapper.get('button').attributes('aria-label')).toBe('钉钉组织管理')
  wrapper.unmount()
})
