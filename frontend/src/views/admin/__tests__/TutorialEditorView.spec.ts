import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

const {
  getTutorialDocument,
  updateTutorialDocument,
  listPageImages,
  uploadPageImage,
  copyToClipboard,
  showSuccess,
  showError,
} = vi.hoisted(() => ({
  getTutorialDocument: vi.fn(),
  updateTutorialDocument: vi.fn(),
  listPageImages: vi.fn(),
  uploadPageImage: vi.fn(),
  copyToClipboard: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

const editorState = vi.hoisted(() => ({
  html: '<p>初始教程</p>',
  setContent: vi.fn((value: string) => {
    editorState.html = value
  }),
  destroy: vi.fn(),
}))

vi.mock('@/api/admin/pages', () => ({
  default: {
    getTutorialDocument,
    updateTutorialDocument,
    listPageImages,
    uploadPageImage,
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showSuccess,
    showError,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('@tiptap/vue-3', () => {
  class MockEditor {
    commands = {
      setContent: editorState.setContent,
    }

    chain() {
      return {
        focus() { return this },
        toggleBold() { return this },
        toggleItalic() { return this },
        toggleUnderline() { return this },
        toggleBulletList() { return this },
        toggleOrderedList() { return this },
        toggleBlockquote() { return this },
        toggleCodeBlock() { return this },
        toggleHeading() { return this },
        setColor() { return this },
        setTextAlign() { return this },
        unsetAllMarks() { return this },
        clearNodes() { return this },
        extendMarkRange() { return this },
        setLink() { return this },
        setImage() { return this },
        run() { return true },
      }
    }

    isActive() {
      return false
    }

    getHTML() {
      return editorState.html
    }

    destroy() {
      editorState.destroy()
    }
  }

  const EditorContent = defineComponent({
    name: 'EditorContentStub',
    props: {
      editor: { type: Object, required: false },
    },
    setup() {
      return () => h('div', { 'data-test': 'editor-content' })
    },
  })

  return {
    Editor: MockEditor,
    EditorContent,
  }
})

vi.mock('@tiptap/starter-kit', () => ({ default: {} }))
vi.mock('@tiptap/extension-link', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-underline', () => ({ default: {} }))
vi.mock('@tiptap/extension-text-style', () => ({ TextStyle: {} }))
vi.mock('@tiptap/extension-color', () => ({ default: {} }))
vi.mock('@tiptap/extension-image', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-text-align', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-placeholder', () => ({ default: { configure: () => ({}) } }))

import TutorialEditorView from '../TutorialEditorView.vue'

describe('admin TutorialEditorView', () => {
  beforeEach(() => {
    editorState.html = '<p>初始教程</p>'
    editorState.setContent.mockClear()
    editorState.destroy.mockClear()

    getTutorialDocument.mockReset()
    updateTutorialDocument.mockReset()
    listPageImages.mockReset()
    uploadPageImage.mockReset()
    copyToClipboard.mockReset()
    showSuccess.mockReset()
    showError.mockReset()

    getTutorialDocument.mockResolvedValue({ content_html: '<p>后端教程</p>' })
    updateTutorialDocument.mockResolvedValue({ content_html: '<p>已保存教程</p>' })
    listPageImages.mockResolvedValue([])
    uploadPageImage.mockResolvedValue({ name: 'demo.png', url: '/api/v1/pages/tutorial/images/demo.png', size: 1024 })
  })

  it('loads tutorial document and saves edited html', async () => {
    const wrapper = mount(TutorialEditorView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
        },
      },
    })

    await flushPromises()

    expect(getTutorialDocument).toHaveBeenCalledTimes(1)
    expect(listPageImages).toHaveBeenCalledWith('tutorial')
    expect(editorState.setContent).toHaveBeenCalledWith('<p>后端教程</p>')
    expect(wrapper.text()).toContain('保存教程文档')
    expect(wrapper.text()).toContain('实时预览')
    expect(wrapper.text()).toContain('H1')
    expect(wrapper.text()).toContain('加粗')

    editorState.html = '<p>新的富文本教程</p>'
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateTutorialDocument).toHaveBeenCalledWith('<p>新的富文本教程</p>')
    expect(showSuccess).toHaveBeenCalledWith('教程文档已保存')
    expect(editorState.setContent).toHaveBeenLastCalledWith('<p>已保存教程</p>')
  })
})
