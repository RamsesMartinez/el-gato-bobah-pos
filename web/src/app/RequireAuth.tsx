import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { Center, Spinner } from '@chakra-ui/react';
import { useSessionStore } from '../stores/session';
import { canAccess } from './roles';

export function RequireAuth({ children }: { children: ReactNode }) {
  const status = useSessionStore((s) => s.status);
  // 'loading' = el arranque aún canjea la cookie de refresh. No rebotar a /login todavía o
  // un reload en frío mandaría al operador a re-loguearse aunque su sesión siga viva.
  if (status === 'loading') {
    return (
      <Center h="100vh">
        <Spinner size="lg" />
      </Center>
    );
  }
  if (status !== 'authed') return <Navigate to="/login" replace />;
  return <>{children}</>;
}

// RequireRole protege una ruta restringida ante un acceso por URL directa: si el rol no
// alcanza, redirige a /pos en vez de dejar que la página dispare un 403. Es UX/defensa en
// profundidad; la autorización real la hace el backend.
export function RequireRole({ path, children }: { path: string; children: ReactNode }) {
  const role = useSessionStore((s) => s.user?.role);
  if (!canAccess(role, path)) return <Navigate to="/pos" replace />;
  return <>{children}</>;
}
