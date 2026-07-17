import { useState } from 'react';
import {
  Box, Heading, Button, HStack, Table, Input, Center, Spinner, Badge,
} from '@chakra-ui/react';
import { NativeSelectRoot, NativeSelectField } from '../../components/ui/native-select';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';

const ROLES = ['admin', 'gerente', 'cajero', 'mesero'];

export function EmployeesPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['users'], queryFn: adminApi.users });
  const [name, setName] = useState('');
  const [role, setRole] = useState('cajero');
  const [pin, setPin] = useState('');

  const create = useMutation({
    mutationFn: () => adminApi.createUser({ name, role, pin: pin || undefined }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] });
      setName(''); setPin('');
      toaster.create({ title: 'Empleado creado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Box p={6} maxW="720px">
      <Heading size="lg" mb={4}>Empleados</Heading>
      <HStack bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px" mb={4} align="end" flexWrap="wrap">
        <Input placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} maxW="200px" />
        <NativeSelectRoot maxW="140px">
          <NativeSelectField value={role} onChange={(e) => setRole(e.target.value)}>
            {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
          </NativeSelectField>
        </NativeSelectRoot>
        <Input placeholder="PIN (4-6 díg.)" value={pin} onChange={(e) => setPin(e.target.value)} maxW="140px" />
        <Button disabled={!name} loading={create.isPending} onClick={() => create.mutate()}>Crear</Button>
      </HStack>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row><Table.ColumnHeader>Nombre</Table.ColumnHeader><Table.ColumnHeader>Usuario</Table.ColumnHeader><Table.ColumnHeader>Rol</Table.ColumnHeader><Table.ColumnHeader>Estado</Table.ColumnHeader></Table.Row></Table.Header>
          <Table.Body>
            {data?.items.map((u) => (
              <Table.Row key={u.id}>
                <Table.Cell>{u.name}</Table.Cell>
                <Table.Cell>{u.username ?? '—'}</Table.Cell>
                <Table.Cell><Badge>{u.role}</Badge></Table.Cell>
                <Table.Cell>{u.isActive ? <Badge colorPalette="green">activo</Badge> : <Badge>inactivo</Badge>}</Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>
    </Box>
  );
}
