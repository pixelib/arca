export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api${path}`, {
    ...init,
    credentials: 'same-origin',
    headers: {
      ...(init?.body ? { 'content-type': 'application/json' } : {}),
      ...init?.headers,
    },
  })

  if (response.status === 204) return undefined as T

  const text = await response.text()
  const payload = text ? (JSON.parse(text) as unknown) : null

  if (!response.ok) {
    const message =
      payload && typeof payload === 'object' && 'error' in payload
        ? String((payload as { error: unknown }).error)
        : `Request failed with status ${response.status}`
    throw new ApiError(response.status, message)
  }

  return payload as T
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
}

export function patch<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: 'PATCH', body: JSON.stringify(body) })
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: 'PUT', body: JSON.stringify(body) })
}

export function del<T>(path: string): Promise<T> {
  return api<T>(path, { method: 'DELETE' })
}

export type Role = 'admin' | 'user'
export type Permission = 'read' | 'write' | 'admin'
export type Policy = 'release' | 'snapshot' | 'mixed'
export type Visibility = 'public' | 'private'
export type RepositoryType = 'hosted' | 'proxy'
export type RepositoryFormat = 'maven2' | 'npm' | 'p2'

export interface User {
  id: string
  email: string
  name: string
  role: Role
}

export interface Build {
  version: string
  commit: string
  buildTime: string
}

export interface Bootstrap {
  initialized: boolean
  instanceName: string
  themeColor: string
  build: Build
  user: User | null
}

export interface InstanceSettings {
  instanceName: string
  themeColor: string
}

export interface Category {
  id: string
  name: string
  description: string
  position: number
  repositories: number
  createdAt: number
}

export interface Repository {
  id: string
  name: string
  format: RepositoryFormat
  type: RepositoryType
  categoryId: string | null
  categoryName: string
  policy: Policy
  visibility: Visibility
  description: string
  allowRedeploy: boolean
  remoteUrl: string
  remoteUsername: string
  metadataTtlSeconds: number
  negativeTtlSeconds: number
  cacheRetentionDays: number
  createdAt: number
  permission: Permission | null
  components?: number
  updatedAt?: number
}

export interface RepositoryDetail extends Repository {
  stats: { components: number; assets: number; bytes: number; updatedAt: number }
}

export interface BrowseEntry {
  name: string
  path: string
  type: 'directory' | 'file'
  size?: number
  updatedAt?: number
}

export interface ArtifactSummary {
  repository: string
  format: RepositoryFormat
  namespace: string
  name: string
  versionCount: number
  latest: string | null
  release: string | null
  updatedAt: number
}

export interface ArtifactDetail {
  repository: string
  format: RepositoryFormat
  namespace: string
  name: string
  latest: string | null
  release: string | null
  versions: Array<{ version: string; isSnapshot: boolean; updatedAt: number; createdAt: number }>
}

export interface ApiToken {
  id: string
  name: string
  preview: string
  created_at: number
  last_used_at: number | null
  expires_at: number | null
}

export interface ManagedUser {
  id: string
  email: string
  name: string
  role: Role
  disabled: number
  created_at: number
}

export interface Grant {
  userId: string
  permission: Permission
  email: string
  name: string
}

export type TrafficRange = '24h' | '7d' | '30d' | '90d'

export interface TrafficTotals {
  downloads: number
  uploads: number
  visits: number
  bytes: number
}

export interface TrafficReport {
  range: TrafficRange
  since: number
  until: number
  interval: number
  buckets: Array<{ at: number; downloads: number; uploads: number; visits: number; bytes: number }>
  totals: TrafficTotals
  previous: TrafficTotals
  topArtifacts: Array<{
    repository: string
    format: RepositoryFormat
    namespace: string
    name: string
    downloads: number
    bytes: number
    lastAt: number
  }>
  repositories: Array<{
    repository: string
    downloads: number
    uploads: number
    bytes: number
  }>
}

export interface StorageTotals {
  repositories: number
  components: number
  assets: number
  bytes: number
  snapshotBytes: number
  releaseBytes: number
}

export interface DiskReport {
  root: string
  files: number
  bytes: number
  trackedBytes: number
  orphanBytes: number
  databaseBytes: number
  measuredAt: number
}

export interface ComponentUsage {
  repository: string
  format: RepositoryFormat
  namespace: string
  name: string
  version: string
  isSnapshot: boolean
  files: number
  bytes: number
  updatedAt: number
}

export interface StorageReport {
  totals: StorageTotals
  repositories: Array<{
    repository: string
    components: number
    assets: number
    bytes: number
    snapshotBytes: number
    releaseBytes: number
  }>
  extensions: Array<{ extension: string; files: number; bytes: number }>
  largest: ComponentUsage[]
  disk: DiskReport
}

export interface HealthReport {
  instanceName: string
  startedAt: number
  uptime: number
  goVersion: string
  goroutines: number
  heapBytes: number
  users: number
  totals: StorageTotals
  disk: DiskReport
  lastDay: TrafficTotals
}
