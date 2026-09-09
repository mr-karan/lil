export type Platform = 'android' | 'ios' | 'web'
export interface ShortURL {
  short_code: string
  url: string
  title?: string
  created_at: string
  expires_at: string | null
  device_urls?: Partial<Record<Platform, { url: string; platform: Platform }>>
}

export interface URLEdit {
  short_code: string
  url: string
  title: string
  device_urls: Record<Platform, string>
}

export async function request(path: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(path, init)
  if (!response.ok) {
    if (response.status === 401) throw new Error('Sign in at /admin/ to manage links.')
    let message = `Request failed (${response.status})`
    try {
      const body: unknown = await response.json()
      if (typeof body === 'object' && body !== null && 'message' in body && typeof body.message === 'string') message = body.message
    } catch { /* Non-JSON responses use the status message. */ }
    throw new Error(message)
  }
  return response
}
