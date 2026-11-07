import { useEffect, useMemo, useState } from 'react';
import {
  Badge, Box, Button, Center, HStack, Input, Spinner, Text, VStack,
} from '@chakra-ui/react';
import { LuChevronDown, LuChevronRight, LuSquare, LuSquareCheck } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminApi, type PermissionInfo, type Role } from '../../api/admin';
import { ApiError } from '../../api/client';
import { mensajeDeError } from '../../api/mensajes';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { Picker } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import {
  ADMIN_FIXED, adjustedText, countText, groupByArea, permissionDiff, productDefault, type AreaGroup,
} from './roleEdit';

interface Props {
  // «+ Rol propio» vive en el encabezado de Empleados, junto a las pestañas.
  creating: boolean;
  onCreatingChange: (v: boolean) => void;
  // Inactivar un rol con personas activas lleva a Personal filtrado por ese rol.
  onShowPeople: (roleId: number) => void;
}

const MAX_ROLE_NAME = 40; // espejo de domain.ValidateRoleName

// RolesPage es la pestaña «Roles» de Empleados (spec 068, opción A): la lista a todo el ancho y cada
// rol en una hoja inferior. Vive dentro de la pantalla de Empleados, así que no trae su propio Page.
export function RolesPage({ creating, onCreatingChange, onShowPeople }: Props) {
  const roles = useQuery({ queryKey: ['roles'], queryFn: adminApi.roles });
  const catalog = useQuery({ queryKey: ['permissions'], queryFn: adminApi.permissionCatalog });
  const [openId, setOpenId] = useState<number | null>(null);

  if (roles.isLoading || catalog.isLoading) return <Center flex="1" minH="200px"><Spinner size="xl" /></Center>;
  if (roles.isError || catalog.isError) {
    return <Text color="fg.error" p={4}>{mensajeDeError(roles.error ?? catalog.error)}</Text>;
  }

  const items = roles.data?.items ?? [];
  const perms = catalog.data?.items ?? [];
  const open = items.find((r) => r.id === openId) ?? null;

  return (
    <Box flex="1" minH={0} overflowY="auto">
      <VStack align="stretch" gap={2} pb={2}>
        {items.map((r) => <RoleRow key={r.id} role={r} onOpen={() => setOpenId(r.id)} />)}
      </VStack>
      {open && (
        <RoleSheet key={open.id} role={open} roles={items} catalog={perms} onClose={() => setOpenId(null)}
          onShowPeople={(id) => { setOpenId(null); onShowPeople(id); }} />
      )}
      {creating && (
        <RoleSheet role={null} roles={items} catalog={perms} onClose={() => onCreatingChange(false)}
          onShowPeople={onShowPeople} />
      )}
    </Box>
  );
}

function RoleRow({ role, onOpen }: { role: Role; onOpen: () => void }) {
  const adjusted = (role.added?.length ?? 0) + (role.removed?.length ?? 0) > 0;
  // Dos chips como máximo: el tipo y un solo estado (inactivo pesa más que ajustado).
  const state = !role.active ? 'inactivo' : role.systemCode && adjusted ? 'ajustado' : null;
  return (
    <Button variant="outline" colorPalette="gray" bg="bg.panel" minH="56px" h="auto" py={2} px={4} w="100%"
      justifyContent="space-between" fontWeight="500" onClick={onOpen}>
      <VStack align="start" gap={0} minW={0}>
        <Text fontWeight="700" truncate>{role.name}</Text>
        <HStack gap={2} flexWrap="wrap">
          <Text fontSize="sm" color="fg.muted">
            {countText(role.activeUsers, 'persona', 'personas')} · {countText(role.permissions?.length ?? 0, 'permiso', 'permisos')}
          </Text>
          <Badge colorPalette={role.systemCode ? 'blue' : 'gray'}>{role.systemCode ? 'sugerido' : 'propio'}</Badge>
          {state && <Badge colorPalette={state === 'inactivo' ? 'yellow' : 'green'}>{state}</Badge>}
        </HStack>
      </VStack>
      <LuChevronRight />
    </Button>
  );
}

interface SheetProps {
  role: Role | null; // null = rol propio nuevo
  roles: Role[];
  catalog: PermissionInfo[];
  onClose: () => void;
  onShowPeople: (roleId: number) => void;
}

function RoleSheet({ role, roles, catalog, onClose, onShowPeople }: SheetProps) {
  const qc = useQueryClient();
  // La hoja monta cerrada y se abre en el render siguiente: si nace abierta mientras otra se cierra
  // en la misma actualización, no se monta (AGENTS.md §3, CobrarSheet).
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const isNew = !role;
  const isAdmin = role?.systemCode === 'admin';
  const isSystem = !!role?.systemCode;
  const original = useMemo(() => role?.permissions ?? [], [role]);
  const [checked, setChecked] = useState(() => new Set(original));
  const [name, setName] = useState(role?.name ?? '');
  const [copyFrom, setCopyFrom] = useState('');
  const [openAreas, setOpenAreas] = useState<Set<string>>(() => new Set());
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState<{ message: string; inUse: boolean } | null>(null);

  const groups = useMemo(() => groupByArea(catalog), [catalog]);
  const defaults = useMemo(
    () => (isSystem && role ? productDefault(original, role.added ?? [], role.removed ?? []) : null),
    [isSystem, role, original],
  );
  const diff = permissionDiff(original, checked);
  const nameChanged = !isNew && !isSystem && name.trim() !== (role?.name ?? '');
  const changes = diff.add.length + diff.remove.length + (nameChanged ? 1 : 0);
  const dirty = isNew ? name.trim() !== '' || checked.size > 0 : changes > 0;

  // Ajuste en vivo de un sugerido contra lo que el producto le da.
  const liveAdded = defaults ? [...checked].filter((c) => !defaults.has(c)).length : 0;
  const liveRemoved = defaults ? [...defaults].filter((c) => !checked.has(c)).length : 0;

  const done = (title: string) => {
    qc.invalidateQueries({ queryKey: ['roles'] });
    toaster.create({ title, type: 'success' });
    onClose();
  };
  const fail = (e: unknown) => {
    // Una versión vieja se resuelve volviendo a abrir el rol: la lista ya trae la nueva.
    if (e instanceof ApiError && e.code === 'ROLE_CHANGED') qc.invalidateQueries({ queryKey: ['roles'] });
    setError({ message: mensajeDeError(e), inUse: e instanceof ApiError && e.code === 'ROLE_IN_USE' });
  };
  const onMut = { onError: fail, onMutate: () => setError(null) };

  const save = useMutation({
    mutationFn: () => {
      if (isNew) return adminApi.createRole({ name: name.trim(), permissions: [...checked] });
      return adminApi.updateRole(role.id, {
        version: role.version,
        ...(nameChanged ? { name: name.trim() } : {}),
        ...(diff.add.length ? { add: diff.add } : {}),
        ...(diff.remove.length ? { remove: diff.remove } : {}),
      });
    },
    onSuccess: () => done(isNew ? 'Rol creado' : 'Rol guardado'),
    ...onMut,
  });
  const reset = useMutation({
    mutationFn: () => adminApi.resetRole(role!.id, role!.version),
    onSuccess: () => done('Rol regresado a lo sugerido'), ...onMut,
  });
  const toggleActive = useMutation({
    mutationFn: () => (role!.active ? adminApi.deactivateRole : adminApi.reactivateRole)(role!.id, role!.version),
    onSuccess: () => done(role!.active ? 'Rol inactivado' : 'Rol reactivado'), ...onMut,
  });
  const remove = useMutation({
    mutationFn: () => adminApi.deleteRole(role!.id),
    onSuccess: () => { setConfirmDelete(false); done('Rol borrado'); },
    onError: (e) => { setConfirmDelete(false); fail(e); },
    onMutate: () => setError(null),
  });

  const requestClose = () => { if (dirty) setConfirmDiscard(true); else onClose(); };

  const toggle = (code: string) => setChecked((prev) => {
    const next = new Set(prev);
    if (next.has(code)) next.delete(code); else next.add(code);
    return next;
  });
  const fixed = (code: string) => isAdmin && code === ADMIN_FIXED;
  const toggleAll = (g: AreaGroup) => setChecked((prev) => {
    const next = new Set(prev);
    const all = g.items.every((p) => next.has(p.code));
    for (const p of g.items) {
      if (fixed(p.code)) continue;
      if (all) next.delete(p.code); else next.add(p.code);
    }
    return next;
  });
  const toggleArea = (area: string) => setOpenAreas((prev) => {
    const next = new Set(prev);
    if (next.has(area)) next.delete(area); else next.add(area);
    return next;
  });

  const copyOptions = roles.map((r) => ({ value: String(r.id), label: r.name, hint: r.active ? undefined : 'inactivo' }));
  const onCopy = (v: string) => {
    setCopyFrom(v);
    const from = roles.find((r) => String(r.id) === v);
    if (from) setChecked(new Set(from.permissions ?? []));
  };

  const adjusted = adjustedText(liveAdded, liveRemoved);
  const canReset = isSystem && !isAdmin && ((role?.added?.length ?? 0) + (role?.removed?.length ?? 0) > 0);
  const canDelete = !isNew && !isSystem && role.allUsers === 0;
  const busy = reset.isPending || toggleActive.isPending || remove.isPending;

  return (
    <>
      <DrawerRoot open={visible} placement="bottom" size="md"
        onOpenChange={(e) => { if (!e.open) requestClose(); }}>
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="92dvh" display="flex" flexDirection="column">
          <DrawerHeader py={3}>
            <HStack justify="space-between" gap={3}>
              <HStack gap={2} minW={0} flexWrap="wrap">
                <Text fontSize="lg" fontWeight="700" truncate>{isNew ? 'Rol propio nuevo' : role.name}</Text>
                {!isNew && <Badge colorPalette={isSystem ? 'blue' : 'gray'}>{isSystem ? 'sugerido' : 'propio'}</Badge>}
                {!isNew && !role.active && <Badge colorPalette="yellow">inactivo</Badge>}
                {adjusted && <Text fontSize="sm" color="fg.muted">{adjusted}</Text>}
              </HStack>
              <Button variant="outline" colorPalette="gray" minH="44px" flexShrink={0} onClick={requestClose}>Cerrar</Button>
            </HStack>
          </DrawerHeader>

          <DrawerBody flex="1" minH={0} overflowY="auto" pt={0}>
            <VStack align="stretch" gap={3}>
              {(isNew || !isSystem) && (
                <HStack data-row="name" gap={3} align="center">
                  <Input minH="44px" flex="1" placeholder="Nombre del rol" aria-label="Nombre del rol"
                    value={name} maxLength={MAX_ROLE_NAME} onChange={(e) => setName(e.target.value)} />
                  {isNew && (
                    <Box flex="1">
                      <Picker value={copyFrom} options={copyOptions} onChange={onCopy}
                        placeholder="Copiar permisos de…" title="Copiar permisos de…" focusSearch={false} />
                    </Box>
                  )}
                </HStack>
              )}

              {groups.map((g) => {
                const n = g.items.filter((p) => checked.has(p.code)).length;
                const expanded = openAreas.has(g.area);
                const all = n === g.items.length;
                return (
                  <Box key={g.area} borderWidth="1px" borderRadius="lg">
                    <HStack gap={2} pr={2}>
                      <Button variant="ghost" colorPalette="gray" flex="1" minH="44px" justifyContent="space-between"
                        aria-expanded={expanded} onClick={() => toggleArea(g.area)}>
                        <HStack gap={2}>
                          {expanded ? <LuChevronDown /> : <LuChevronRight />}
                          <Text fontWeight="700">{g.name}</Text>
                        </HStack>
                        <Text fontSize="sm" color="fg.muted">{n} de {g.items.length}</Text>
                      </Button>
                      {expanded && (
                        <Button variant="outline" colorPalette="gray" size="sm" minH="44px" onClick={() => toggleAll(g)}>
                          {all ? 'Quitar todas' : 'Marcar todas'}
                        </Button>
                      )}
                    </HStack>
                    {expanded && (
                      <VStack align="stretch" gap={1} px={2} pb={2}>
                        {g.items.map((p) => {
                          const on = checked.has(p.code);
                          const tag = defaults && on && !defaults.has(p.code) ? 'agregado'
                            : defaults && !on && defaults.has(p.code) ? 'quitado' : null;
                          return (
                            <Box key={p.code}>
                              <Button role="checkbox" aria-checked={on} disabled={fixed(p.code)}
                                variant="ghost" colorPalette="gray" w="100%" minH="44px" justifyContent="start" gap={3}
                                fontWeight="500" onClick={() => toggle(p.code)}>
                                {on ? <LuSquareCheck /> : <LuSquare />}
                                <Text flex="1" textAlign="start" truncate>{p.label}</Text>
                                {tag && <Badge colorPalette={tag === 'agregado' ? 'green' : 'orange'}>{tag}</Badge>}
                              </Button>
                              {fixed(p.code) && (
                                <Text fontSize="sm" color="fg.muted" pl={10}>El administrador siempre puede administrar roles</Text>
                              )}
                            </Box>
                          );
                        })}
                      </VStack>
                    )}
                  </Box>
                );
              })}

              {error && (
                <Box borderWidth="1px" borderColor="border.error" borderRadius="lg" p={3}>
                  <Text color="fg.error">{error.message}</Text>
                  {error.inUse && role && (
                    <Button mt={2} minH="44px" variant="outline" onClick={() => onShowPeople(role.id)}>Ver a esas personas</Button>
                  )}
                </Box>
              )}

              {!isNew && (canReset || !isAdmin || canDelete) && (
                <HStack gap={3} pt={4} mt={2} borderTopWidth="1px" flexWrap="wrap">
                  {canReset && (
                    <Button variant="outline" colorPalette="gray" minH="44px" disabled={busy} loading={reset.isPending}
                      onClick={() => reset.mutate()}>Volver a lo sugerido</Button>
                  )}
                  {!isAdmin && (
                    <Button variant="outline" colorPalette="gray" minH="44px" disabled={busy} loading={toggleActive.isPending}
                      onClick={() => toggleActive.mutate()}>{role.active ? 'Inactivar' : 'Reactivar'}</Button>
                  )}
                  {canDelete && (
                    <Button variant="outline" colorPalette="red" minH="44px" ml="auto" disabled={busy}
                      onClick={() => setConfirmDelete(true)}>Borrar</Button>
                  )}
                </HStack>
              )}
            </VStack>
          </DrawerBody>

          <DrawerFooter borderTopWidth="1px" py={3}>
            <HStack w="100%" justify="space-between">
              <Text color="fg.muted">
                {isNew ? countText(checked.size, 'permiso', 'permisos') : countText(changes, 'cambio', 'cambios')}
              </Text>
              <Button minH="44px" px={6} loading={save.isPending}
                disabled={isNew ? name.trim() === '' : changes === 0}
                onClick={() => save.mutate()}>{isNew ? 'Crear' : 'Guardar'}</Button>
            </HStack>
          </DrawerFooter>
        </DrawerContent>
      </DrawerRoot>

      <ConfirmSheet isOpen={confirmDiscard} title="¿Descartar los cambios?" destructive
        description="Lo que marcaste o quitaste en este rol no se guarda."
        confirmLabel="Descartar" cancelLabel="Seguir editando"
        onConfirm={() => { setConfirmDiscard(false); onClose(); }} onCancel={() => setConfirmDiscard(false)} />
      {role && (
        <ConfirmSheet isOpen={confirmDelete} title={`¿Borrar el rol ${role.name}?`} destructive
          description="Nadie tiene este rol. Se borra y no se puede deshacer."
          confirmLabel="Borrar" cancelLabel="Volver" loading={remove.isPending}
          onConfirm={() => remove.mutate()} onCancel={() => setConfirmDelete(false)} />
      )}
    </>
  );
}
