import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { Center, Spinner } from '@chakra-ui/react';
import { useSessionStore } from '../stores/session';

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
