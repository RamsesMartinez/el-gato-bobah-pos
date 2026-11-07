import type { ReactNode } from 'react';
import { Navigate } from 'react-router';
import { Center, Spinner } from '@chakra-ui/react';
import { useSessionStore } from '../stores/session';
import { canOpen } from './routes';

export function RequireAuth({ children }: { children: ReactNode }) {
  const status = useSessionStore((s) => s.status);
  // 'loading' = el arranque aún canjea la cookie de refresh. No rebotar a /login todavía o
  // un reload en frío mandaría al operador a re-loguearse aunque su sesión siga viva.
  if (status === 'loading') {
    return (
      <Center h="100dvh">
        <Spinner size="lg" />
      </Center>
    );
  }
  if (status !== 'authed') return <Navigate to="/login" replace />;
  return <>{children}</>;
}

// RequirePermission protege una ruta restringida ante un acceso por URL directa: sin el permiso
// redirige a /pos en vez de dejar que la página dispare un 403. Es UX/defensa en profundidad; la
// autorización real la hace el backend.
export function RequirePermission({ path, children }: { path: string; children: ReactNode }) {
  const user = useSessionStore((s) => s.user);
  if (!canOpen(path, user)) return <Navigate to="/pos" replace />;
  return <>{children}</>;
}
