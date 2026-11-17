import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Logo } from '@/components/brand'
import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/input'
import { ErrorBlock } from '@/components/ui/feedback'
import { ApiError, post } from '@/lib/api'
import { useRefreshSession, useSession } from '@/lib/session'

// What the OIDC callback means by each ?error= code it redirects here with. The
// setup wizard shows them too, since App sends /login there before setup.
export const oidcErrors: Record<string, string> = {
  denied: 'Your account is not allowed to sign in here.',
  email_taken: 'An account with your email already exists. Sign in with your password instead.',
  not_admin: 'Only members of an administrator group can set up this instance.',
  failed: 'Single sign-on could not be completed. Try again.',
}

export function LoginRoute() {
  const { instanceName, oidc } = useSession()
  const [searchParams] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(() => oidcErrors[searchParams.get('error') ?? ''] ?? null)
  const [busy, setBusy] = useState(false)
  const refresh = useRefreshSession()
  const navigate = useNavigate()

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post('/auth/login', { email: email.trim().toLowerCase(), password })
      await refresh()
      navigate('/', { replace: true })
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Sign in could not be completed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm space-y-8">
        <div className="space-y-5 text-center">
          <Link to="/" className="inline-flex items-center gap-2.5">
            <Logo className="size-6" />
            <span className="text-lg font-semibold tracking-tight">{instanceName}</span>
          </Link>
          <h1 className="text-2xl font-semibold">Sign in</h1>
        </div>

        <form onSubmit={submit} className="space-y-5 rounded-3xl bg-card p-7 shadow-card dark:ring-1 dark:ring-border">
          {error ? <ErrorBlock message={error} /> : null}
          <Field label="Email">
            <Input
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="username"
              className="font-mono"
              autoFocus
            />
          </Field>
          <Field label="Password">
            <Input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
            />
          </Field>
          <Button type="submit" size="lg" className="w-full" disabled={busy || !email || !password}>
            {busy ? 'Signing in' : 'Sign in'}
          </Button>
        </form>

        {oidc ? (
          <Button asChild variant="outline" size="lg" className="w-full">
            <a href="/api/auth/oidc/login">Sign in with {oidc.name}</a>
          </Button>
        ) : null}

        <p className="text-center text-sm text-muted-foreground">
          <Link to="/" className="hover:text-foreground">
            Browse public repositories instead
          </Link>
        </p>
      </div>
    </div>
  )
}
