import { useRef } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { AjustesDeCaja } from './AjustesDeCaja';
import { ToleranciaDeDiferencia } from './ToleranciaDeDiferencia';
import { seccionDeCaja, rutaDeCaja } from './rutasDeCaja';
import { Heading, VStack, Tabs, Text } from '@chakra-ui/react';
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
  const canOperate = can('cash.operate', user);
  const navigate = useNavigate();
  const location = useLocation();
  // La pestaña, la caja y el corte viven en la dirección (#290, FR-017): una recarga vuelve a donde
  // se estaba y «corte #410» se puede compartir.
  const seccion = seccionDeCaja(location.pathname, canManage || canBusiness);
  // La caja elegida sobrevive a ir y volver de otra pestaña: volver a «Cajas» con otra caja en
  // pantalla es operar la equivocada.
  // Se actualiza al salir de «Cajas», en el manejador y no al pintar.
  const ultimaCaja = useRef<number | null>(null);
  const irA = (pestana: typeof seccion.pestana) => {
    if (seccion.pestana === 'operar') ultimaCaja.current = seccion.cajaId;
    navigate(pestana === 'operar' && ultimaCaja.current !== null ? `/caja/${ultimaCaja.current}` : rutaDeCaja(pestana));
  };
  const cajaNoEncontrada = (location.state as { cajaNoEncontrada?: boolean } | null)?.cajaNoEncontrada === true;
  return (
    // Sin tope de ancho (#290, FR-036): en 1920 y 2560 px el contenido usa la pantalla. Lo que se
    // limita es el largo de las líneas de texto, en cada pieza.
    <Page maxW="none">
      <Heading size="lg" mb={4}>Caja</Heading>
      {cajaNoEncontrada && <Text color="fg.muted" mb={2} role="status">No encontramos esa caja.</Text>}
      <Tabs.Root value={seccion.pestana} onValueChange={(e) => irA(e.value as typeof seccion.pestana)}>
        <Tabs.List>
          {/* 44 px: medían 40. Sin operar la caja (quien solo configura el negocio) no hay Cajas ni
              Histórico que ofrecer: el servidor los negaría. */}
          {canOperate && <Tabs.Trigger value="operar" minH="44px">Cajas</Tabs.Trigger>}
          {canOperate && <Tabs.Trigger value="historico" minH="44px">Histórico</Tabs.Trigger>}
          {(canManage || canBusiness) && <Tabs.Trigger value="gestion" minH="44px">Administrar</Tabs.Trigger>}
        </Tabs.List>
        <Tabs.Content value="operar" px={0} pt={4}>
          <RegistersTab cajaId={seccion.cajaId} onCaja={(id) => navigate(`/caja/${id}`)}
            cerrando={seccion.cerrando}
            onCerrando={(id, v) => navigate(v ? `/caja/${id}/cerrar` : `/caja/${id}`)}
            onCajaNoEncontrada={() => { ultimaCaja.current = null; navigate('/caja', { replace: true, state: { cajaNoEncontrada: true } }); }} />
        </Tabs.Content>
        <Tabs.Content value="historico" px={0} pt={4}>
          <HistoryTab corteId={seccion.corteId}
            onCorte={(id) => (id === null
              // Cerrar el diálogo reemplaza la entrada: «Atrás» no debe reabrir el corte recién cerrado.
              ? navigate('/caja/historico', { replace: true })
              : navigate(`/caja/historico/${id}`))} />
        </Tabs.Content>
        {(canManage || canBusiness) && (
          <Tabs.Content value="gestion" px={0} pt={4}>
            <VStack align="stretch" gap={6}>
              {canManage && <ManageRegistersTab />}
              {canManage && <ToleranciaDeDiferencia />}
              {canBusiness && <AjustesDeCaja />}
            </VStack>
          </Tabs.Content>
        )}
      </Tabs.Root>
    </Page>
  );
}
