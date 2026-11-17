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

export function upload<T>(path: string, file: File): Promise<T> {
  return api<T>(path, {
    method: 'PUT',
    body: file,
    headers: { 'content-type': file.type || 'application/octet-stream' },
  })
}

export function del<T>(path: string): Promise<T> {
  return api<T>(path, { method: 'DELETE' })
}

export type Role = 'admin' | 'user'
export type Permission = 'read' | 'write' | 'admin'
export type Policy = 'release' | 'snapshot' | 'mixed'
export type Visibility = 'public' | 'private'
export type RepositoryType = 'hosted' | 'proxy' | 'group'
export type RepositoryFormat = 'maven2' | 'npm' | 'p2' | 'docker'

export type MigrationState = 'idle' | 'running' | 'done' | 'cancelled' | 'failed'
export type MigrationRepositoryState = 'pending' | 'running' | 'done' | 'failed' | 'skipped'

export interface MigrationRepository {
  name: string
  source: string
  action: 'create' | 'skip'
  format: string
  type: string
  reason: string
  state?: MigrationRepositoryState
  existed?: boolean
  copied: number
  present: number
  failed: number
  untranslatable: number
  copyingAssets: boolean
}

export interface MigrationStatus {
  state: MigrationState
  source: string
  startedAt: number
  finishedAt: number
  current: string
  message: string
  repositories: MigrationRepository[]
  errors: string[]
}

export interface MigrationPreview {
  repositories: MigrationRepository[]
}

export interface MigrationRequest {
  url: string
  username: string
  password: string
  visibility: Visibility
  rawFormat: RepositoryFormat
}

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
  welcomeText: string
  logoUrl: string
  build: Build
  user: User | null
  oidc: { name: string } | null
  // Who signed in through the provider before setup, to become the administrator.
  setupIdentity: { name: string; email: string } | null
}

export interface InstanceSettings {
  instanceName: string
  themeColor: string
  welcomeText: string
  logoUrl: string
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
  /** Former names that still redirect here, newest first. */
  aliases: string[]
  components?: number
  updatedAt?: number
}

export interface RepositoryDetail extends Repository {
  stats: { components: number; assets: number; bytes: number; updatedAt: number }
  members?: string[]
}

export interface RepositoryAlias {
  name: string
  createdAt: number
}

export type MatchType = 'template' | 'regex'

/** What a deployment link points at right now, or null when nothing matches. */
export interface ResolvedFile {
  version: string
  filename: string
  path: string
  size: number
  updatedAt: number
}

export interface DeploymentLink {
  id: string
  slug: string
  url: string
  repository: string
  description: string
  namespace: string
  artifact: string
  matchType: MatchType
  pattern: string
  policy: Policy
  filename: string
  disabled: boolean
  downloads: number
  lastUsedAt: number
  createdAt: number
  resolved: ResolvedFile | null
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

export interface DockerLayer {
  digest: string
  short: string
  mediaType: string
  size: number
  position: number
  sharedWith: number
  foreign: boolean
  urls: string[]
  stored: boolean
}

export interface DockerChild {
  digest: string
  short: string
  mediaType: string
  platform: string
  size: number
  totalSize: number
  os: string
  architecture: string
  variant: string
  layerCount: number
  indexed: boolean
  referenceType: string
}

export interface DockerHistoryStep {
  created: number
  createdBy: string
  comment: string
  emptyLayer: boolean
}

export interface DockerConfig {
  digest: string
  size: number
  user: string
  workingDir: string
  entrypoint: string[]
  cmd: string[]
  env: string[]
  exposedPorts: string[]
  history: DockerHistoryStep[]
}

export interface DockerManifest {
  repository: string
  namespace: string
  name: string
  image: string
  reference: string
  digest: string
  short: string
  mediaType: string
  size: number
  totalSize: number
  layerCount: number
  isIndex: boolean
  os: string
  architecture: string
  variant: string
  imageCreated: number
  labels: Record<string, string>
  annotations: Record<string, string>
  layers: DockerLayer[]
  children: DockerChild[]
  config: DockerConfig | null
  pullBy: { tag: string; digest: string }
}

export interface DockerReindexReport {
  repositories: Array<{ repository: string; manifests: number; failed: number }>
}

export interface DockerSweepReport {
  repositories: Array<{
    repository: string
    blobs: number
    manifests: number
    bytes: number
  }>
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
  // Linked to the identity provider.
  sso: boolean
  // The provider's admin groups set this account's role, so it cannot be changed here.
  role_managed: boolean
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
