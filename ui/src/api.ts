export type Platform = 'android' | 'ios' | 'web'
export interface ShortURL {
  short_code: string
  url: string
  title?: string
  created_at: string
  expires_at: string | null
  created_by: UserRef | null
  updated_by: UserRef | null
  updated_at: string | null
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
    if (response.status === 401) {
      window.location.assign('/auth/login?next=' + encodeURIComponent(location.pathname + location.search))
      throw new Error('Redirecting to sign in')
    }
    let message = `Request failed (${response.status})`
    try {
      const body: unknown = await response.json()
      if (typeof body === 'object' && body !== null && 'message' in body && typeof body.message === 'string') message = body.message
    } catch { /* Non-JSON responses use the status message. */ }
    throw new Error(message)
  }
  return response
}

export interface UserRef {
  id: number
  email: string
  name: string
}

export interface User extends UserRef {
  created_at: string
  last_login_at: string | null
  disabled_at: string | null
}

export interface APIToken {
  id: number
  name: string
  token_prefix: string
  created_at: string
}

export const auditActions = [
  'url.create',
  'url.update',
  'url.delete',
  'token.create',
  'token.revoke',
  'user.disable',
  'user.enable',
] as const
export type AuditAction = (typeof auditActions)[number]

export interface AuditEntry {
  id: number
  created_at: string
  actor: UserRef
  token_id: number | null
  action: AuditAction
  target: string
  before: unknown
  after: unknown
}

export interface AuditPage {
  entries: AuditEntry[]
  page: number
  per_page: number
  count: number
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === 'string'
}

function isAuditAction(value: unknown): value is AuditAction {
  return auditActions.some(action => action === value)
}

function isUserRef(value: unknown): value is UserRef {
  return isRecord(value) && typeof value.id === 'number' && typeof value.email === 'string' && typeof value.name === 'string'
}

function isUser(value: unknown): value is User {
  return isUserRef(value) && isRecord(value) && typeof value.created_at === 'string' &&
    isNullableString(value.last_login_at) && isNullableString(value.disabled_at)
}

function isAPIToken(value: unknown): value is APIToken {
  return isRecord(value) && typeof value.id === 'number' && typeof value.name === 'string' &&
    typeof value.token_prefix === 'string' && typeof value.created_at === 'string'
}

function isAuditEntry(value: unknown): value is AuditEntry {
  return isRecord(value) && typeof value.id === 'number' && typeof value.created_at === 'string' &&
    isUserRef(value.actor) && (value.token_id === null || typeof value.token_id === 'number') &&
    isAuditAction(value.action) && typeof value.target === 'string'
}

function isListOf<T>(value: unknown, guard: (item: unknown) => item is T): value is T[] {
  return Array.isArray(value) && value.every(guard)
}

function isAuditPage(value: unknown): value is AuditPage {
  return isRecord(value) && isListOf(value.entries, isAuditEntry) && typeof value.page === 'number' &&
    typeof value.per_page === 'number' && typeof value.count === 'number'
}

// Reads a JSON body and unwraps the {"status", "data"} envelope when present.
async function readPayload(response: Response): Promise<unknown> {
  const body: unknown = await response.json()
  return isRecord(body) && 'data' in body ? body.data : body
}

async function requestPayload<T>(path: string, guard: (value: unknown) => value is T, init?: RequestInit): Promise<T> {
  const payload = await readPayload(await request(path, init))
  if (!guard(payload)) throw new Error('Unexpected response from server')
  return payload
}

export function displayName(user: UserRef | null): string {
  if (!user) return 'unknown'
  return user.name || user.email
}

export function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}

export const getMe = () => requestPayload('/api/v1/me', isUser)
export const getUsers = () => requestPayload('/api/v1/users', (v): v is User[] => isListOf(v, isUser))
export const setUserDisabled = (id: number, disabled: boolean) =>
  request(`/api/v1/users/${id}/${disabled ? 'disable' : 'enable'}`, { method: 'POST' })
export const getTokens = () => requestPayload('/api/v1/tokens', (v): v is APIToken[] => isListOf(v, isAPIToken))
export const revokeToken = (id: number) => request(`/api/v1/tokens/${id}`, { method: 'DELETE' })
export const getAudit = (query: URLSearchParams) => requestPayload(`/api/v1/audit?${query}`, isAuditPage)

export async function createToken(name: string): Promise<{ token: string; api_token: APIToken }> {
  const isCreated = (v: unknown): v is { token: string; api_token: APIToken } =>
    isRecord(v) && typeof v.token === 'string' && isAPIToken(v.api_token)
  return requestPayload('/api/v1/tokens', isCreated, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}
