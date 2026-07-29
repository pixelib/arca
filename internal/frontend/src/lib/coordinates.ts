import type { RepositoryFormat } from '@/lib/api'

export function packageLabel(
  format: RepositoryFormat,
  namespace: string,
  name: string,
): string {
  if (format === 'npm') return namespace ? `@${namespace}/${name}` : name
  return `${namespace}:${name}`
}

export function coordinateLabel(
  format: RepositoryFormat,
  namespace: string,
  name: string,
  version?: string | null,
): string {
  const label = packageLabel(format, namespace, name)
  if (!version) return label
  return format === 'npm' ? `${label}@${version}` : `${label}:${version}`
}

export function formatLabel(format: RepositoryFormat): string {
  if (format === 'npm') return 'npm'
  if (format === 'p2') return 'p2'
  return 'Maven'
}
