import { apiClient } from '../client'

export interface AdminPageContent {
  slug: string
  content_md: string
}

export interface TutorialDocumentPayload {
  content_html: string
}

export interface AdminPageImage {
  name: string
  url: string
  size: number
}

async function getPageContent(slug: string): Promise<string> {
  const { data } = await apiClient.get<string>(`/pages/${encodeURIComponent(slug)}`, {
    responseType: 'text' as const,
  })
  return typeof data === 'string' ? data : ''
}

async function updatePageContent(slug: string, contentMD: string): Promise<AdminPageContent> {
  const { data } = await apiClient.put<AdminPageContent>(`/pages/${encodeURIComponent(slug)}`, {
    content_md: contentMD,
  })
  return data
}

async function listPageImages(slug: string): Promise<AdminPageImage[]> {
  const { data } = await apiClient.get<AdminPageImage[]>(`/pages/${encodeURIComponent(slug)}/images`)
  return Array.isArray(data) ? data : []
}

async function uploadPageImage(slug: string, file: File): Promise<AdminPageImage> {
  const formData = new FormData()
  formData.append('file', file)
  const { data } = await apiClient.post<AdminPageImage>(`/pages/${encodeURIComponent(slug)}/images`, formData)
  return data
}

async function getTutorialDocument(): Promise<TutorialDocumentPayload> {
  const { data } = await apiClient.get<TutorialDocumentPayload>('/tutorial-document')
  return data
}

async function updateTutorialDocument(contentHTML: string): Promise<TutorialDocumentPayload> {
  const { data } = await apiClient.put<TutorialDocumentPayload>('/tutorial-document', {
    content_html: contentHTML,
  })
  return data
}

export default {
  getPageContent,
  updatePageContent,
  listPageImages,
  uploadPageImage,
  getTutorialDocument,
  updateTutorialDocument,
}
