import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Download, Folder, Globe, Lock, ArrowUpRight } from 'lucide-react'
import { Badge, FormatBadge, PolicyBadge, TypeBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EmptyState, ErrorBlock, LoadingBlock } from '@/components/ui/feedback'
import { Coordinate, CopyButton } from '@/components/copy-button'
import { MavenUsage } from '@/components/snippets/maven'
import { NPMUsage } from '@/components/snippets/npm'
import { P2Usage } from '@/components/snippets/p2'
import {
  api,
  type ArtifactSummary,
  type BrowseEntry,
  type RepositoryDetail,
  type RepositoryFormat,
} from '@/lib/api'
import { formatBytes, formatRelative } from '@/lib/utils'

interface BrowseResponse {
  path: string
  entries: BrowseEntry[]
  artifact: { namespace: string; name: string } | null
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl bg-muted px-4 py-3.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className="mt-1 text-lg font-semibold tabular-nums">{value}</p>
    </div>
  )
}

function Breadcrumb({ repository, path }: { repository: string; path: string }) {
  const segments = path.split('/').filter(Boolean)
  return (
    <nav className="flex flex-wrap items-center gap-1 font-mono text-xs text-muted-foreground">
      <Link to={`/repositories/${repository}`} className="hover:text-foreground">
        {repository}
      </Link>
      {segments.map((segment, index) => (
        <span key={`${segment}-${index}`} className="flex items-center gap-1">
          <ChevronRight className="size-3 text-faint" />
          <Link
            to={`/repositories/${repository}/${segments.slice(0, index + 1).join('/')}`}
            className={index === segments.length - 1 ? 'text-foreground' : 'hover:text-foreground'}
          >
            {segment}
          </Link>
        </span>
      ))}
    </nav>
  )
}

function Browser({
  repository,
  path,
  format,
  isProxy,
}: {
  repository: string
  path: string
  format: RepositoryFormat
  isProxy: boolean
}) {
  const { data, error, isLoading } = useQuery({
    queryKey: ['browse', repository, path],
    queryFn: () =>
      api<BrowseResponse>(`/repositories/${repository}/browse?path=${encodeURIComponent(path)}`),
  })

  if (isLoading) return <LoadingBlock label="Loading directory" />
  if (error) return <ErrorBlock message={(error as Error).message} />
  if (!data) return null

  if (data.entries.length === 0) {
    return isProxy ? (
      <EmptyState
        title="Nothing cached yet"
        description="Resolve a dependency through this repository and the cached tree appears here."
      />
    ) : (
      <EmptyState
        title="Nothing published yet"
        description={
          format === 'npm'
            ? 'Run npm publish against this registry and the tree appears here immediately.'
            : 'Deploy with Maven and the tree appears here immediately.'
        }
      />
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Breadcrumb repository={repository} path={path} />
        {data.artifact ? (
          <Button size="sm" variant="outline" asChild>
            <Link
              to={`/artifacts/${repository}?namespace=${encodeURIComponent(data.artifact.namespace)}&name=${encodeURIComponent(data.artifact.name)}`}
            >
              Open artifact <ArrowUpRight />
            </Link>
          </Button>
        ) : null}
      </div>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-28">Size</TableHead>
              <TableHead className="w-36">Updated</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.entries.map((entry) => (
              <TableRow key={entry.path}>
                <TableCell className="font-mono text-[13px]">
                  {entry.type === 'directory' ? (
                    <Link
                      to={`/repositories/${repository}/${entry.path.replace(/\/$/, '')}`}
                      className="flex items-center gap-2 hover:text-primary"
                    >
                      <Folder className="size-3.5 text-faint" />
                      {entry.name}
                    </Link>
                  ) : (
                    <a
                      href={`/repository/${repository}/${entry.path}`}
                      className="flex items-center gap-2 hover:text-primary"
                    >
                      <span aria-hidden className="size-1.5 rounded-full bg-faint" />
                      {entry.name}
                    </a>
                  )}
                </TableCell>
                <TableCell className="font-mono text-xs tabular-nums text-muted-foreground">
                  {entry.type === 'file' ? formatBytes(entry.size ?? 0) : ''}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {entry.type === 'file' ? formatRelative(entry.updatedAt) : ''}
                </TableCell>
                <TableCell>
                  {entry.type === 'file' ? (
                    <a
                      href={`/repository/${repository}/${entry.path}`}
                      download
                      aria-label={`Download ${entry.name}`}
                      className="inline-flex size-6 items-center justify-center text-faint transition-colors hover:bg-accent hover:text-foreground"
                    >
                      <Download className="size-3" />
                    </a>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  )
}

function Overview({
  repository,
  format,
  isProxy,
}: {
  repository: string
  format: RepositoryFormat
  isProxy: boolean
}) {
  const { data, error, isLoading } = useQuery({
    queryKey: ['recent', repository],
    queryFn: () => api<{ artifacts: ArtifactSummary[] }>(`/repositories/${repository}/recent`),
  })

  if (isLoading) return <LoadingBlock label="Loading artifacts" />
  if (error) return <ErrorBlock message={(error as Error).message} />

  const artifacts = data?.artifacts ?? []
  if (artifacts.length === 0) {
    return isProxy ? (
      <EmptyState
        title="No artifacts cached yet"
        description="Point a build at this repository and the artifacts it resolves are cached here. The Usage tab has the exact configuration."
      />
    ) : (
      <EmptyState
        title="No artifacts published yet"
        description={
          format === 'npm'
            ? 'Point an npm publish at this registry to get started. The Usage tab has the exact configuration.'
            : 'Point a Maven deploy at this repository to get started. The Usage tab has the exact configuration.'
        }
      />
    )
  }

  return (
    <Card>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Artifact</TableHead>
            <TableHead className="w-44">Latest</TableHead>
            <TableHead className="w-24">Versions</TableHead>
            <TableHead className="w-36">Updated</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {artifacts.map((artifact) => (
            <TableRow key={`${artifact.namespace}:${artifact.name}`}>
              <TableCell>
                <Link
                  to={`/artifacts/${repository}?namespace=${encodeURIComponent(artifact.namespace)}&name=${encodeURIComponent(artifact.name)}`}
                  className="block"
                >
                  <Coordinate
                    format={artifact.format}
                    namespace={artifact.namespace}
                    name={artifact.name}
                    className="text-[13px]"
                  />
                </Link>
              </TableCell>
              <TableCell className="font-mono text-[13px] text-primary">{artifact.latest}</TableCell>
              <TableCell className="font-mono text-xs tabular-nums text-muted-foreground">
                {artifact.versionCount}
              </TableCell>
              <TableCell className="text-xs text-muted-foreground">
                {formatRelative(artifact.updatedAt)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function Usage({ repository }: { repository: RepositoryDetail }) {
  if (repository.format === 'npm') return <NPMUsage repository={repository} />
  if (repository.format === 'p2') return <P2Usage repository={repository} />
  return <MavenUsage repository={repository} />
}

export function RepositoryRoute() {
  const params = useParams()
  const name = params.name!
  const path = params['*'] ?? ''

  const { data, error, isLoading } = useQuery({
    queryKey: ['repository', name],
    queryFn: () => api<RepositoryDetail>(`/repositories/${name}`),
  })

  if (isLoading) return <LoadingBlock label="Loading repository" />
  if (error) return <ErrorBlock message={(error as Error).message} />
  if (!data) return null

  const endpoint = `${window.location.origin}/repository/${data.name}`

  return (
    <>
      <div className="space-y-6">
        <div className="space-y-2.5">
          <p className="text-[13px] font-medium text-primary">Repository</p>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="font-mono text-3xl font-semibold">{data.name}</h1>
            <FormatBadge format={data.format} />
            {data.type === 'proxy' ? <TypeBadge type={data.type} /> : null}
            <PolicyBadge policy={data.policy} />
            <Badge variant={data.visibility === 'public' ? 'primary' : 'neutral'}>
              {data.visibility === 'public' ? (
                <Globe className="size-3" />
              ) : (
                <Lock className="size-3" />
              )}
              {data.visibility}
            </Badge>
            {data.permission ? <Badge variant="neutral">{data.permission}</Badge> : null}
          </div>
          {data.description ? (
            <p className="max-w-2xl text-[15px] text-muted-foreground">{data.description}</p>
          ) : null}
        </div>

        <div className="flex items-center gap-3 rounded-2xl bg-tint py-2 pl-5 pr-2 text-tint-foreground">
          <code className="min-w-0 flex-1 truncate font-mono text-[13px]">{endpoint}</code>
          <CopyButton value={endpoint} className="shrink-0 hover:bg-card/60" />
        </div>

        {data.type === 'proxy' ? (
          <p className="text-sm text-muted-foreground">
            Mirrors <code className="font-mono text-[13px]">{data.remoteUrl}</code>. Artifacts are
            cached on first request, and the tree below shows what has been cached so far.
            {data.cacheRetentionDays > 0
              ? ` Cached files are deleted after ${data.cacheRetentionDays} days without a download.`
              : ''}
          </p>
        ) : null}

        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label={data.type === 'proxy' ? 'Cached artifacts' : 'Artifacts'} value={String(data.stats.components)} />
          <Stat label="Files" value={String(data.stats.assets)} />
          <Stat label="Storage" value={formatBytes(data.stats.bytes)} />
          <Stat
            label={data.type === 'proxy' ? 'Last cached' : 'Last publish'}
            value={formatRelative(data.stats.updatedAt)}
          />
        </div>
      </div>

      <Tabs defaultValue={path ? 'browse' : 'overview'}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="browse">Browse</TabsTrigger>
          <TabsTrigger value="usage">Usage</TabsTrigger>
        </TabsList>
        <TabsContent value="overview">
          <Overview repository={name} format={data.format} isProxy={data.type === 'proxy'} />
        </TabsContent>
        <TabsContent value="browse">
          <Browser
            repository={name}
            path={path}
            format={data.format}
            isProxy={data.type === 'proxy'}
          />
        </TabsContent>
        <TabsContent value="usage">
          <Usage repository={data} />
        </TabsContent>
      </Tabs>
    </>
  )
}
