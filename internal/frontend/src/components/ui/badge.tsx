import type * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import type { RepositoryFormat } from '@/lib/api'
import { formatLabel } from '@/lib/coordinates'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium leading-none',
  {
    variants: {
      variant: {
        neutral: 'bg-muted text-muted-foreground',
        primary: 'bg-primary-soft text-primary',
        release: 'bg-release-soft text-release',
        snapshot: 'bg-snapshot-soft text-snapshot',
        danger: 'bg-destructive-soft text-destructive',
      },
    },
    defaultVariants: { variant: 'neutral' },
  },
)

export function Badge({
  className,
  variant,
  ...props
}: React.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />
}

export function TypeBadge({ type }: { type: string }) {
  return <Badge variant={type === 'proxy' ? 'primary' : 'neutral'}>{type}</Badge>
}

export function FormatBadge({ format }: { format: RepositoryFormat }) {
  return (
    <Badge variant={format === 'maven2' ? 'neutral' : 'primary'}>{formatLabel(format)}</Badge>
  )
}

export function PolicyBadge({ policy }: { policy: string }) {
  return (
    <Badge variant={policy === 'snapshot' ? 'snapshot' : policy === 'mixed' ? 'neutral' : 'release'}>
      {policy}
    </Badge>
  )
}
