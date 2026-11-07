import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  Badge, Box, Button, Center, Flex, Grid, HStack, Input, Spinner, Text, VStack,
} from '@chakra-ui/react';
import { LuCheck, LuPlus, LuSquare, LuSquareCheck } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminApi, type PermissionInfo, type Role } from '../../api/admin';
import { ApiError } from '../../api/client';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { mensajeDeError } from '../../api/mensajes';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { Picker } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import {
  ADMIN_FIXED, adjustedText, countText, fitsIn, groupByArea, permissionDiff, productDefault, type AreaGroup,
} from './roleEdit';

interface Props {
  // Las pestañas de Empleados van arriba de la lista: así el rol elegido tiene el alto entero.
  tabs?: ReactNode;
  // Inactivar un rol con personas activas lleva a Personal filtrado por ese rol.
  onShowPeople: (roleId: number) => void;
}

const MAX_ROLE_NAME = 40; // espejo de domain.ValidateRoleName

type Selection = number | 'new';

// RolesPage es la pestaña «Roles» de Empleados (#243, opción B «Dos paneles»): la lista a la izquierda
// y el rol elegido a la derecha, sin hoja que tape la lista; cambiar de rol es un toque. Vive dentro
// de la pantalla de Empleados, así que no trae su propio Page.
export function RolesPage({ tabs, onShowPeople }: Props) {
  const roles = useQuery({ queryKey: ['roles'], queryFn: adminApi.roles });
  const catalog = useQuery({ queryKey: ['permissions'], queryFn: adminApi.permissionCatalog });
  const [picked, setPicked] = useState<Selection | null>(null);
  // generation vuelve a montar el editor con el rol recargado tras guardar: el editor congela el rol
  // al abrirlo y no se entera solo de lo que la lista trae nuevo.
  const [generation, setGeneration] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [pending, setPending] = useState<Selection | null>(null);
  const onDirty = useCallback((d: boolean) => setDirty(d), []);

  if (roles.isLoading || catalog.isLoading) return <Center flex="1" minH="200px"><Spinner size="xl" /></Center>;
  if (roles.isError || catalog.isError) {
    return <Text color="fg.error" p={4}>{mensajeDeError(roles.error ?? catalog.error)}</Text>;
  }

  const items = roles.data?.items ?? [];
  const perms = catalog.data?.items ?? [];
  // Un rol que ya no está (se borró) deja elegido el primero.
  const selected: Selection | null = picked === 'new' || items.some((r) => r.id === picked) ? picked : items[0]?.id ?? null;
  const role = typeof selected === 'number' ? items.find((r) => r.id === selected) ?? null : null;

  const choose = (next: Selection) => {
    if (next === selected) return;
    if (dirty) { setPending(next); return; }
    setPicked(next);
  };
  const reopen = (next: Selection | null) => {
    setDirty(false);
    setPicked(next);
    setGeneration((g) => g + 1);
  };

  return (
    <Flex direction={{ base: 'column', md: 'row' }} gap={3} flex="1" minH={0}>
      <Flex direction="column" gap={2} w={{ base: '100%', md: '300px' }} flexShrink={0} minH={0}>
        <HStack gap={2} justify="space-between" flexWrap="wrap">
          {tabs}
          <Button minH="44px" ml="auto" onClick={() => choose('new')}><LuPlus /> Rol propio</Button>
        </HStack>
        <Box data-role-list flex="1" minH={0} overflowY="auto" maxH={{ base: '35dvh', md: 'none' }}
          bg="bg.panel" borderWidth="1px" borderRadius="lg">
          {items.map((r) => <RoleRow key={r.id} role={r} current={r.id === selected} onOpen={() => choose(r.id)} />)}
          {selected === 'new' && (
            <Box minH="56px" px={4} display="flex" alignItems="center" borderLeftWidth="4px"
              borderLeftColor="brand.solid" bg="brand.subtle">
              <Text fontWeight="700">Rol propio nuevo</Text>
            </Box>
          )}
        </Box>
      </Flex>

      {selected === null ? (
        <Text color="fg.muted" p={4}>No hay roles.</Text>
      ) : (
        <RoleEditor key={`${selected}-${generation}`} role={selected === 'new' ? null : role} roles={items}
          catalog={perms} onDirty={onDirty} onDone={reopen} onShowPeople={onShowPeople} />
      )}

      <ConfirmSheet isOpen={pending !== null} title="¿Descartar los cambios?" destructive
        description="Lo que marcaste o quitaste en este rol no se guarda."
        confirmLabel="Descartar" cancelLabel="Seguir editando"
        onConfirm={() => { const next = pending; setPending(null); setDirty(false); setPicked(next); }}
        onCancel={() => setPending(null)} />
    </Flex>
  );
}

function RoleRow({ role, current, onOpen }: { role: Role; current: boolean; onOpen: () => void }) {
  const adjusted = (role.added?.length ?? 0) + (role.removed?.length ?? 0) > 0;
  // Dos chips como máximo: el tipo y un solo estado (inactivo pesa más que ajustado).
  const state = !role.active ? 'inactivo' : role.systemCode && adjusted ? 'ajustado' : null;
  return (
    <Button data-role-row variant="ghost" colorPalette="gray" minH="56px" h="auto" py={2} px={4} w="100%"
      justifyContent="start" fontWeight="500" borderRadius="0" borderStyle="solid" borderBottomWidth="1px"
      borderLeftWidth="4px" borderLeftColor={current ? 'brand.solid' : 'transparent'}
      bg={current ? 'brand.subtle' : undefined} aria-current={current ? 'true' : undefined} onClick={onOpen}>
      <VStack align="start" gap={0} minW={0}>
        <Text fontWeight="700" truncate maxW="250px">{role.name}</Text>
        <HStack gap={2} flexWrap="wrap">
          <Text fontSize="sm" color="fg.muted">
            {countText(role.activeUsers, 'persona', 'personas')} · {countText(role.permissions?.length ?? 0, 'permiso', 'permisos')}
          </Text>
          <Badge colorPalette={role.systemCode ? 'blue' : 'gray'}>{role.systemCode ? 'sugerido' : 'propio'}</Badge>
          {state && <Badge colorPalette={state === 'inactivo' ? 'yellow' : 'green'}>{state}</Badge>}
        </HStack>
      </VStack>
    </Button>
  );
}

interface EditorProps {
  role: Role | null; // null = rol propio nuevo
  roles: Role[];
  catalog: PermissionInfo[];
  onDirty: (dirty: boolean) => void;
  // onDone: terminó un guardado o una acción; elige `next` (null = el primero) recargado.
  onDone: (next: Selection | null) => void;
  onShowPeople: (roleId: number) => void;
}

function RoleEditor({ role: liveRole, roles, catalog, onDirty, onDone, onShowPeople }: EditorProps) {
  const qc = useQueryClient();
  // El rol tal como se mostró al abrir: la lista ['roles'] se vuelve a pedir sola, y comparar contra
  // la recargada mandaría como «quitado» lo que otra persona agregó mientras tanto, con la versión
  // nueva, y el servidor lo aceptaría.
  const [role] = useState(liveRole);
  const user = useSessionStore((s) => s.user);
  const mine = user?.permissions;
  const held = (code: string) => mine?.includes(code) ?? false;

  const isNew = !role;
  const isAdmin = role?.systemCode === 'admin';
  const isSystem = !!role?.systemCode;
  // Un rol que puede más que la sesión se ve pero no se toca: el servidor lo rechazaría.
  const readOnly = !!role && !fitsIn(role.permissions ?? [], mine);
  const original = useMemo(() => role?.permissions ?? [], [role]);
  const [checked, setChecked] = useState(() => new Set(original));
  const [name, setName] = useState(role?.name ?? '');
  const [copyFrom, setCopyFrom] = useState('');
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  // where: el error se pinta junto a lo que lo provocó; el de una acción del final no se ve arriba.
  const [error, setError] = useState<{ message: string; inUse: boolean; where: 'save' | 'action' } | null>(null);

  const groups = useMemo(() => groupByArea(catalog), [catalog]);
  const defaults = useMemo(
    () => (isSystem && role ? productDefault(original, role.added ?? [], role.removed ?? []) : null),
    [isSystem, role, original],
  );
  const diff = permissionDiff(original, checked);
  const nameChanged = !isNew && !isSystem && name.trim() !== (role?.name ?? '');
  const nameBlank = (isNew || !isSystem) && name.trim() === '';
  const changes = diff.add.length + diff.remove.length + (nameChanged ? 1 : 0);
  const dirty = isNew ? name.trim() !== '' || checked.size > 0 : changes > 0;
  // La lista pregunta antes de cambiar de rol con cambios sin guardar.
  useEffect(() => { onDirty(dirty); }, [dirty, onDirty]);

  // Ajuste en vivo de un sugerido contra lo que el producto le da.
  const liveAdded = defaults ? [...checked].filter((c) => !defaults.has(c)).length : 0;
  const liveRemoved = defaults ? [...defaults].filter((c) => !checked.has(c)).length : 0;

  const done = async (title: string, next: Selection | null) => {
    toaster.create({ title, type: 'success' });
    await qc.invalidateQueries({ queryKey: ['roles'] });
    onDone(next);
  };
  const fail = (where: 'save' | 'action') => async (e: unknown) => {
    // Lo marcado se hizo sobre otra versión del rol: se descarta y se muestra como quedó.
    if (e instanceof ApiError && e.code === 'ROLE_CHANGED') {
      toaster.create({ title: 'Alguien cambió este rol; ya se ve como quedó', type: 'warning' });
      await qc.invalidateQueries({ queryKey: ['roles'] });
      onDone(role ? role.id : 'new');
      return;
    }
    setError({ message: mensajeDeError(e), inUse: e instanceof ApiError && e.code === 'ROLE_IN_USE', where });
  };
  const onAction = { onError: fail('action'), onMutate: () => setError(null) };

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
    onSuccess: (r) => done(isNew ? 'Rol creado' : 'Rol guardado', isNew ? r.id : role.id),
    onError: fail('save'),
    onMutate: () => setError(null),
  });
  const reset = useMutation({
    mutationFn: () => adminApi.resetRole(role!.id, role!.version),
    onSuccess: () => done('Rol regresado a lo sugerido', role!.id), ...onAction,
  });
  const toggleActive = useMutation({
    mutationFn: () => (role!.active ? adminApi.deactivateRole : adminApi.reactivateRole)(role!.id, role!.version),
    onSuccess: () => done(role!.active ? 'Rol inactivado' : 'Rol reactivado', role!.id), ...onAction,
  });
  const remove = useMutation({
    mutationFn: () => adminApi.deleteRole(role!.id),
    onSuccess: () => { setConfirmDelete(false); return done('Rol borrado', null); },
    onError: (e) => { setConfirmDelete(false); return fail('action')(e); },
    onMutate: () => setError(null),
  });

  const toggle = (code: string) => setChecked((prev) => {
    const next = new Set(prev);
    if (next.has(code)) next.delete(code); else next.add(code);
    return next;
  });
  const fixed = (code: string) => isAdmin && code === ADMIN_FIXED;
  const locked = (code: string) => readOnly || fixed(code) || !held(code);
  const toggleAll = (g: AreaGroup) => setChecked((prev) => {
    const next = new Set(prev);
    const all = g.items.filter((p) => !locked(p.code)).every((p) => next.has(p.code));
    for (const p of g.items) {
      if (locked(p.code)) continue;
      if (all) next.delete(p.code); else next.add(p.code);
    }
    return next;
  });

  const copyOptions = roles.map((r) => ({ value: String(r.id), label: r.name, hint: r.active ? undefined : 'inactivo' }));
  const onCopy = (v: string) => {
    setCopyFrom(v);
    const from = roles.find((r) => String(r.id) === v);
    if (from) setChecked(new Set((from.permissions ?? []).filter(held)));
  };

  const adjusted = adjustedText(liveAdded, liveRemoved);
  const canReset = isSystem && !isAdmin && !readOnly && !!defaults && fitsIn(defaults, mine)
    && ((role?.added?.length ?? 0) + (role?.removed?.length ?? 0) > 0);
  const canDelete = !isNew && !isSystem && role.allUsers === 0;
  const usedBefore = !isNew && !isSystem && role.allUsers > 0;
  const canToggle = !isAdmin && !readOnly;
  const busy = reset.isPending || toggleActive.isPending || remove.isPending;

  const errorBox = (where: 'save' | 'action') => error?.where === where && (
    <Box borderWidth="1px" borderColor="border.error" borderRadius="lg" p={3}>
      <Text color="fg.error">{error.message}</Text>
      {error.inUse && role && can('employees.manage', user) && (
        <Button mt={2} minH="44px" variant="outline" onClick={() => onShowPeople(role.id)}>Ver a esas personas</Button>
      )}
    </Box>
  );

  return (
    <Flex as="section" aria-label="Permisos del rol" direction="column" flex="1" minW={0} minH={0}
      bg="bg.panel" borderWidth="1px" borderRadius="lg">
      <HStack gap={3} px={3} py={1.5} borderBottomWidth="1px" justify="space-between">
        <HStack data-row="name" gap={2} minW={0} flex="1" flexWrap="wrap">
          {isNew ? (
            <>
              <Input minH="44px" flex="1" minW="150px" placeholder="Nombre del rol" aria-label="Nombre del rol"
                value={name} maxLength={MAX_ROLE_NAME} onChange={(e) => setName(e.target.value)} />
              <Box flex="1" minW="150px">
                <Picker value={copyFrom} options={copyOptions} onChange={onCopy}
                  placeholder="Copiar permisos de…" title="Copiar permisos de…" focusSearch={false} />
              </Box>
            </>
          ) : isSystem ? (
            <Text fontSize="lg" fontWeight="700" truncate>{role.name}</Text>
          ) : (
            <Input minH="44px" maxW="220px" placeholder="Nombre del rol" aria-label="Nombre del rol"
              value={name} readOnly={readOnly} maxLength={MAX_ROLE_NAME} onChange={(e) => setName(e.target.value)} />
          )}
          {!isNew && <Badge colorPalette={isSystem ? 'blue' : 'gray'}>{isSystem ? 'sugerido' : 'propio'}</Badge>}
          {!isNew && !role.active && <Badge colorPalette="yellow">inactivo</Badge>}
          {adjusted && <Text fontSize="sm" color="fg.muted">{adjusted}</Text>}
        </HStack>
        {!readOnly && (
          <HStack gap={3} flexShrink={0}>
            <Text color="fg.muted" fontSize="sm">
              {isNew ? countText(checked.size, 'permiso', 'permisos') : countText(changes, 'cambio', 'cambios')}
            </Text>
            <Button minH="44px" px={6} loading={save.isPending}
              disabled={nameBlank || (!isNew && changes === 0)}
              onClick={() => save.mutate()}>{isNew ? 'Crear' : 'Guardar'}</Button>
          </HStack>
        )}
      </HStack>
      {readOnly && (
        <Text fontSize="sm" color="fg.muted" px={3} py={2} borderBottomWidth="1px">
          Este rol tiene permisos que tu usuario no tiene. Solo puedes verlo.
        </Text>
      )}
      {error?.where === 'save' && <Box px={3} pt={2}>{errorBox('save')}</Box>}

      <Box data-editor-scroll flex="1" minH={0} overflowY="auto" px={3} pb={3}>
        {groups.map((g) => {
          const n = g.items.filter((p) => checked.has(p.code)).length;
          const all = n === g.items.length;
          return (
            <Box key={g.area} role="group" aria-label={g.name}>
              <HStack gap={2} minH="44px" justify="space-between">
                <HStack gap={2}>
                  <Text fontWeight="700">{g.name}</Text>
                  <Text fontSize="sm" color="fg.muted">{n} de {g.items.length}</Text>
                </HStack>
                {!readOnly && (
                  <Button variant="ghost" colorPalette="gray" size="sm" minH="44px" onClick={() => toggleAll(g)}>
                    {all ? 'Quitar todas' : 'Marcar todas'}
                  </Button>
                )}
              </HStack>
              <Grid data-testid="permisos-del-area" templateColumns="repeat(2, minmax(0, 1fr))" columnGap={2} rowGap="2px" mb={1}>
                {g.items.map((p) => {
                  const on = checked.has(p.code);
                  const tag = defaults && on && !defaults.has(p.code) ? 'agregado'
                    : defaults && !on && defaults.has(p.code) ? 'quitado' : null;
                  return (
                    <Box key={p.code}>
                      <Button data-perm={p.code} role="checkbox" aria-checked={on} disabled={locked(p.code)}
                        variant="ghost" colorPalette="gray" w="100%" minH="44px" h="auto" py={0} justifyContent="start" gap={3}
                        fontWeight="500" whiteSpace="normal" bg={on ? 'brand.subtle' : undefined}
                        onClick={() => toggle(p.code)}>
                        <Box as="span" color={on ? 'brand.solid' : undefined} flexShrink={0}>
                          {on ? <LuSquareCheck /> : <LuSquare />}
                        </Box>
                        <Text flex="1" textAlign="start" lineClamp={2} lineHeight="1.2">{p.label}</Text>
                        {tag && <Badge colorPalette={tag === 'agregado' ? 'green' : 'orange'}>{tag}</Badge>}
                      </Button>
                      {fixed(p.code) && (
                        <Text fontSize="sm" color="fg.muted" pl={10}>El administrador siempre puede administrar roles</Text>
                      )}
                      {!readOnly && !held(p.code) && (
                        <Text fontSize="sm" color="fg.muted" pl={10}>Tu usuario no lo tiene</Text>
                      )}
                    </Box>
                  );
                })}
                {g.area === 'orders' && (
                  <HStack gap={2} minH="44px" px={3} borderWidth="1px" borderStyle="dashed" borderRadius="md" color="fg.muted">
                    <LuCheck />
                    <Text fontSize="sm">Tomar pedidos y cobrar lo puede cualquier rol</Text>
                  </HStack>
                )}
              </Grid>
            </Box>
          );
        })}

        {!isNew && (canReset || canToggle) && (
          <VStack align="stretch" gap={2} pt={3} mt={3} borderTopWidth="1px">
            <HStack gap={3} flexWrap="wrap">
              {canReset && (
                <Button variant="outline" colorPalette="gray" minH="44px" disabled={busy || dirty} loading={reset.isPending}
                  onClick={() => setConfirmReset(true)}>Volver a lo sugerido</Button>
              )}
              {canToggle && (
                <Button variant="outline" colorPalette="gray" minH="44px" disabled={busy || dirty} loading={toggleActive.isPending}
                  onClick={() => toggleActive.mutate()}>{role.active ? 'Inactivar' : 'Reactivar'}</Button>
              )}
              {canDelete && (
                <Button variant="outline" colorPalette="red" minH="44px" ml="auto" disabled={busy}
                  onClick={() => setConfirmDelete(true)}>Borrar</Button>
              )}
              {usedBefore && (
                <Text fontSize="sm" color="fg.muted" ml="auto">Alguien lo tiene asignado (aunque esté dado de baja): solo se puede inactivar</Text>
              )}
            </HStack>
            {dirty && <Text fontSize="sm" color="fg.muted">Guarda o descarta los cambios primero</Text>}
            {errorBox('action')}
          </VStack>
        )}
      </Box>

      {role && (
        <ConfirmSheet isOpen={confirmReset} title={`¿Regresar ${role.name} a lo sugerido?`}
          description="Se quitan los permisos que le agregaron y vuelven los que le quitaron."
          confirmLabel="Regresar a lo sugerido" cancelLabel="Cancelar" loading={reset.isPending}
          onConfirm={() => { setConfirmReset(false); reset.mutate(); }} onCancel={() => setConfirmReset(false)} />
      )}
      {role && (
        <ConfirmSheet isOpen={confirmDelete} title={`¿Borrar el rol ${role.name}?`} destructive
          description="Nadie tiene este rol. Se borra y no se puede deshacer."
          confirmLabel="Borrar" cancelLabel="Cancelar" loading={remove.isPending}
          onConfirm={() => remove.mutate()} onCancel={() => setConfirmDelete(false)} />
      )}
    </Flex>
  );
}
