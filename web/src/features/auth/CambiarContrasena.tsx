import { useState } from 'react';
import { Button, Input, VStack } from '@chakra-ui/react';
import { posApi } from '../../api/pos';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useSessionStore } from '../../stores/session';

const MIN_LEN = 12;

// Formulario para cambiar la propia contraseña. Lo usan «Mi cuenta» y la pantalla del cambio
// obligatorio; al terminar apaga el indicador de la sesión, que el servidor ya apagó.
export function CambiarContrasena({ onChanged }: { onChanged?: () => void }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [saving, setSaving] = useState(false);

  const valid = current !== '' && next.length >= MIN_LEN && !next.includes('@') && next === confirm && next !== current;

  const save = async () => {
    setSaving(true);
    try {
      await posApi.changeOwnPassword(current, next);
      toaster.create({ title: 'Contraseña actualizada', type: 'success' });
      setCurrent(''); setNext(''); setConfirm('');
      const user = useSessionStore.getState().user;
      if (user?.mustChangePassword) useSessionStore.setState({ user: { ...user, mustChangePassword: false } });
      onChanged?.();
    } catch (err) {
      toaster.create({ title: 'No se pudo cambiar', description: err instanceof ApiError ? err.message : String(err), type: 'error' });
    } finally {
      setSaving(false);
    }
  };

  return (
    <VStack align="stretch" gap={3}>
      <Input type="password" autoComplete="current-password" placeholder="Contraseña actual" value={current} onChange={(e) => setCurrent(e.target.value)} />
      <Input type="password" autoComplete="new-password" placeholder="Nueva contraseña (mín. 12)" value={next} onChange={(e) => setNext(e.target.value)} />
      <Input type="password" autoComplete="new-password" placeholder="Repetir nueva contraseña" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
      <Button onClick={save} disabled={!valid} loading={saving}>Cambiar contraseña</Button>
    </VStack>
  );
}
