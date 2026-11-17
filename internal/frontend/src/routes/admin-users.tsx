import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, Input, Select } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ErrorBlock, LoadingBlock, PageHeading } from '@/components/ui/feedback'
import { ApiError, api, del, patch, post, type ManagedUser, type Role } from '@/lib/api'
import { useSession } from '@/lib/session'
import { formatDate } from '@/lib/utils'

function CreateUserDialog() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<Role>('user')
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: () => post('/users', { name, email, password, role }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['users'] })
      setOpen(false)
      setName('')
      setEmail('')
      setPassword('')
    },
  })

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus /> New user
        </Button>
      </DialogTrigger>
      <DialogContent>
        <form
          onSubmit={(event: FormEvent) => {
            event.preventDefault()
            mutation.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>New user</DialogTitle>
            <DialogDescription>They can change this password once signed in.</DialogDescription>
          </DialogHeader>
          <DialogBody>
            {mutation.error ? <ErrorBlock message={(mutation.error as ApiError).message} /> : null}
            <Field label="Full name">
              <Input value={name} onChange={(event) => setName(event.target.value)} autoFocus />
            </Field>
            <Field label="Email" hint="Also the Maven username.">
              <Input
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                className="font-mono"
              />
            </Field>
            <Field label="Initial password" hint="At least 10 characters.">
              <Input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </Field>
            <Field label="Role">
              <Select value={role} onChange={(event) => setRole(event.target.value as Role)}>
                <option value="user">User</option>
                <option value="admin">Administrator</option>
              </Select>
            </Field>
          </DialogBody>
          <DialogFooter>
            <Button
              type="submit"
              disabled={!name || !email || password.length < 10 || mutation.isPending}
            >
              Create user
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function AdminUsersRoute() {
  const { user: currentUser, oidc } = useSession()
  const queryClient = useQueryClient()
  const { data, error, isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => api<{ users: ManagedUser[] }>('/users'),
  })

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Record<string, unknown> }) =>
      patch(`/users/${id}`, body),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['users'] }),
  })

  const remove = useMutation({
    mutationFn: (id: string) => del(`/users/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['users'] }),
  })

  if (currentUser?.role !== 'admin') return <ErrorBlock message="Administrator access required" />
  if (isLoading) return <LoadingBlock label="Loading users" />
  if (error) return <ErrorBlock message={(error as Error).message} />

  return (
    <>
      <PageHeading
        eyebrow="Manage"
        title="Users"
        description="Accounts that can sign in and hold repository permissions."
        actions={<CreateUserDialog />}
      />

      {update.error ? <ErrorBlock message={(update.error as ApiError).message} /> : null}
      {remove.error ? <ErrorBlock message={(remove.error as ApiError).message} /> : null}

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Email</TableHead>
              <TableHead className="w-40">Role</TableHead>
              <TableHead className="w-28">Status</TableHead>
              <TableHead className="w-32">Created</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(data?.users ?? []).map((user) => (
              <TableRow key={user.id}>
                <TableCell className="font-medium">
                  {user.name}
                  {user.sso ? (
                    <Badge
                      variant="primary"
                      className="ml-2 size-5 justify-center p-0 align-middle"
                      role="img"
                      aria-label={`Signs in through ${oidc?.name}`}
                      title={`Signs in through ${oidc?.name}`}
                    >
                      <KeyRound className="size-3" />
                    </Badge>
                  ) : null}
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {user.email}
                </TableCell>
                <TableCell>
                  <Select
                    className="h-8 w-36 text-xs"
                    value={user.role}
                    disabled={user.id === currentUser.id || user.role_managed}
                    title={user.role_managed ? `Set by ${oidc?.name} groups` : undefined}
                    onChange={(event) =>
                      update.mutate({ id: user.id, body: { role: event.target.value } })
                    }
                  >
                    <option value="user">User</option>
                    <option value="admin">Administrator</option>
                  </Select>
                </TableCell>
                <TableCell>
                  <button
                    type="button"
                    disabled={user.id === currentUser.id}
                    className="disabled:cursor-not-allowed disabled:opacity-60"
                    onClick={() =>
                      update.mutate({ id: user.id, body: { disabled: !user.disabled } })
                    }
                  >
                    <Badge variant={user.disabled ? 'danger' : 'release'}>
                      {user.disabled ? 'disabled' : 'active'}
                    </Badge>
                  </button>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {formatDate(user.created_at)}
                </TableCell>
                <TableCell>
                  <button
                    type="button"
                    disabled={user.id === currentUser.id}
                    aria-label={`Delete ${user.email}`}
                    onClick={() => remove.mutate(user.id)}
                    className="inline-flex size-6 items-center justify-center text-faint transition-colors hover:bg-accent hover:text-destructive disabled:pointer-events-none disabled:opacity-40"
                  >
                    <Trash2 className="size-3" />
                  </button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </>
  )
}
