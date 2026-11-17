import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { AppShell } from '@/components/app-shell'
import { LoadingBlock } from '@/components/ui/feedback'
import { SessionProvider, useSession } from '@/lib/session'
import { AccountRoute } from '@/routes/account'
import { AdminArtifactsRoute } from '@/routes/admin-artifacts'
import { AdminCategoriesRoute } from '@/routes/admin-categories'
import { AdminHealthRoute } from '@/routes/admin-health'
import { AdminMaintenanceRoute } from '@/routes/admin-maintenance'
import { AdminRepositoriesRoute } from '@/routes/admin-repositories'
import { AdminSettingsRoute } from '@/routes/admin-settings'
import { AdminStorageRoute } from '@/routes/admin-storage'
import { AdminUsersRoute } from '@/routes/admin-users'
import { ArtifactRoute } from '@/routes/artifact'
import { LoginRoute } from '@/routes/login'
import { RepositoriesRoute } from '@/routes/repositories'
import { RepositoryRoute } from '@/routes/repository'
import { SearchRoute } from '@/routes/search'
import { SetupRoute } from '@/routes/setup'

function Router() {
  const { initialized, user } = useSession()
  const location = useLocation()

  if (!initialized) {
    // The search is kept so an OIDC ?error= reaches the wizard.
    return location.pathname === '/setup' ? (
      <SetupRoute />
    ) : (
      <Navigate to={{ pathname: '/setup', search: location.search }} replace />
    )
  }

  if (location.pathname === '/setup') return <Navigate to="/" replace />
  if (location.pathname === '/login') {
    return user ? <Navigate to="/" replace /> : <LoginRoute />
  }

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<RepositoriesRoute />} />
        <Route path="repositories/:name/*" element={<RepositoryRoute />} />
        <Route path="artifacts/:name" element={<ArtifactRoute />} />
        <Route path="search" element={<SearchRoute />} />
        <Route path="account" element={<AccountRoute />} />
        <Route path="admin/health" element={<AdminHealthRoute />} />
        <Route path="admin/storage" element={<AdminStorageRoute />} />
        <Route path="admin/artifacts" element={<AdminArtifactsRoute />} />
        <Route path="admin/repositories" element={<AdminRepositoriesRoute />} />
        <Route path="admin/categories" element={<AdminCategoriesRoute />} />
        <Route path="admin/users" element={<AdminUsersRoute />} />
        <Route path="admin/settings" element={<AdminSettingsRoute />} />
        <Route path="admin/maintenance" element={<AdminMaintenanceRoute />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

export function App() {
  return (
    <SessionProvider
      fallback={
        <div className="flex min-h-dvh items-center justify-center">
          <LoadingBlock label="Starting Arca" />
        </div>
      }
    >
      <Router />
    </SessionProvider>
  )
}
