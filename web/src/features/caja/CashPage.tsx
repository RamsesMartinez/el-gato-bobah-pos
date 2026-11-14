import { useLocation, useNavigate } from 'react-router';
import { AjustesDeCaja } from './AjustesDeCaja';
import { seccionDeCaja, rutaDeCaja } from './rutasDeCaja';
import { Heading, VStack, Tabs } from '@chakra-ui/react';
import { Page } from '../../components/Page';
import { useSessionStore } from '../../stores/session';
import { can } from '../../app/permissions';
import { ManageRegistersTab } from './administrar';
import { HistoryTab } from './reporte';
import { RegistersTab } from './turno';

export function CashPage() {
  const user = useSessionStore((s) => s.user);
  const canManage = can('cash.configure', user);
  // Los ajustes de terminales, modo de arqueo y correos piden configurar el negocio: se muestran a
  // quien antes los veía en Negocio, ni a más ni a menos.
  const canBusiness = can('business.configure', user);
  const navigate = useNavigate();
  // La pestaña, la caja y el corte viven en la dirección (#290, FR-017): una recarga vuelve a donde
  // se estaba y «corte #410» se puede compartir.
  const seccion = seccionDeCaja(useLocation().pathname, canManage || canBusiness);
  return (
    <Page maxW="920px">
      <Heading size="lg" mb={4}>Caja</Heading>
      <Tabs.Root value={seccion.pestana} onValueChange={(e) => navigate(rutaDeCaja(e.value as typeof seccion.pestana))}>
        <Tabs.List>
          {/* 44 px: medían 40. */}
          <Tabs.Trigger value="operar" minH="44px">Cajas</Tabs.Trigger>
          <Tabs.Trigger value="historico" minH="44px">Histórico</Tabs.Trigger>
          {(canManage || canBusiness) && <Tabs.Trigger value="gestion" minH="44px">Administrar</Tabs.Trigger>}
        </Tabs.List>
        <Tabs.Content value="operar" px={0} pt={4}>
          <RegistersTab cajaId={seccion.cajaId} onCaja={(id) => navigate(`/caja/${id}`)} />
        </Tabs.Content>
        <Tabs.Content value="historico" px={0} pt={4}>
          <HistoryTab corteId={seccion.corteId}
            onCorte={(id) => navigate(id === null ? '/caja/historico' : `/caja/historico/${id}`)} />
        </Tabs.Content>
        {(canManage || canBusiness) && (
          <Tabs.Content value="gestion" px={0} pt={4}>
            <VStack align="stretch" gap={6}>
              {canManage && <ManageRegistersTab />}
              {canBusiness && <AjustesDeCaja />}
            </VStack>
          </Tabs.Content>
        )}
      </Tabs.Root>
    </Page>
  );
}
