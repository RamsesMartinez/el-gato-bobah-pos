import type { ReactNode } from 'react';
import { Box, Button, Checkbox, HStack, Input, Text, Textarea, VStack } from '@chakra-ui/react';
import { Field } from '../../components/ui/field';
import { Switch } from '../../components/ui/switch';
import { moneyExact } from '../../utils/format';
import type { VentaDelProducto } from './ventaDelProducto';

interface PanelProps {
  venta: VentaDelProducto;
  setVenta: (v: VentaDelProducto) => void;
}

// LugarDeVenta es una casilla grande: toda la tarjeta es el objetivo del dedo (≥ 56 px).
export function LugarDeVenta({ label, nota, checked, onChange }: { label: string; nota: string; checked: boolean; onChange: (c: boolean) => void }) {
  return (
    <Box flex="1" borderWidth="1px" borderRadius="lg" borderColor={checked ? 'brand.solid' : 'border'} bg={checked ? 'brand.subtle' : undefined}>
      <Checkbox.Root size="lg" minH="56px" w="100%" px={3} checked={checked} onCheckedChange={(e) => onChange(e.checked === true)}>
        <Checkbox.HiddenInput />
        <Checkbox.Control />
        <Checkbox.Label>
          <Text fontWeight="600">{label}</Text>
          <Text fontSize="xs" color="fg.muted" fontWeight="normal">{nota}</Text>
        </Checkbox.Label>
      </Checkbox.Root>
    </Box>
  );
}

// LugaresDeVenta: las casillas Mostrador y Web, encendidas por omisión (spec 092).
export function LugaresDeVenta({ venta, setVenta }: PanelProps) {
  return (
    <HStack align="stretch" gap={3}>
      <LugarDeVenta label="Mostrador" nota="Sale en Vender" checked={venta.counter} onChange={(c) => setVenta({ ...venta, counter: c })} />
      <LugarDeVenta label="Web" nota="Sale en el menú en línea" checked={venta.web} onChange={(c) => setVenta({ ...venta, web: c })} />
    </HStack>
  );
}

// PanelVentaEnWeb junta lo del producto para la web: si se vende ahí, a qué precio y la descripción
// para el cliente. Con la web apagada se queda el interruptor solo; lo capturado se conserva.
export function PanelVentaEnWeb({ venta, setVenta, precioMostrador, errorPrecio }: PanelProps & { precioMostrador: string; errorPrecio: boolean }) {
  return (
    <VStack align="stretch" gap={5} maxW="640px">
      <HStack justify="space-between" align="start">
        <Box pr={3}>
          <Text>Se vende en la web</Text>
          <Text fontSize="xs" color="fg.muted">Sale en el menú en línea.</Text>
        </Box>
        <Switch size="lg" py="10px" checked={venta.web} inputProps={{ 'aria-label': 'Se vende en la web' }}
          onCheckedChange={(e) => setVenta({ ...venta, web: e.checked })} />
      </HStack>
      {venta.web && (
        <>
          <VStack align="stretch" gap={2}>
            <Checkbox.Root size="lg" minH="44px" checked={venta.usarMostrador}
              onCheckedChange={(e) => setVenta({ ...venta, usarMostrador: e.checked === true })}>
              <Checkbox.HiddenInput />
              <Checkbox.Control />
              <Checkbox.Label>Usar el precio de mostrador ({moneyExact(precioMostrador)})</Checkbox.Label>
            </Checkbox.Root>
            {!venta.usarMostrador && (
              <Field label="Precio en la web" invalid={errorPrecio} errorText="Escribe el precio en la web, mayor que cero"
                helperText={`En mostrador sigue en ${moneyExact(precioMostrador)}`}>
                <Input type="number" inputMode="decimal" minH="44px" value={venta.precioWeb}
                  onChange={(e) => setVenta({ ...venta, precioWeb: e.target.value })} />
              </Field>
            )}
          </VStack>
          <Field label="Descripción para el cliente" helperText="Se ve en el menú en línea.">
            <Textarea rows={3} minH="88px" maxLength={500} value={venta.description}
              placeholder="Qué lleva, para cuántos alcanza…"
              onChange={(e) => setVenta({ ...venta, description: e.target.value })} />
          </Field>
        </>
      )}
    </VStack>
  );
}

// VenderSinExistencia: solo para lo que descuenta del almacén; si no, la pantalla dice por qué no
// está la opción en vez de esconderla en silencio.
export function VenderSinExistencia({ venta, setVenta, mide }: PanelProps & { mide: boolean }) {
  if (!mide) {
    return <Text fontSize="sm" color="fg.muted">Este producto no descuenta del almacén. Agrega su receta para controlarlo.</Text>;
  }
  return (
    <HStack justify="space-between" align="start">
      <Box pr={3}>
        <Text>Vender sin existencia</Text>
        <Text fontSize="xs" color="fg.muted">Apagado, cuando se acaba sale «Agotado» en mostrador y en la web.</Text>
      </Box>
      <Switch size="lg" py="10px" checked={venta.allowOversell} inputProps={{ 'aria-label': 'Vender sin existencia' }}
        onCheckedChange={(e) => setVenta({ ...venta, allowOversell: e.checked })} />
    </HStack>
  );
}

export interface Pestana {
  id: string;
  label: string;
}

// MenuDePestanas es el menú lateral de la ficha a pantalla completa (maqueta G2). Un punto rojo marca
// la pestaña que tiene algo por corregir.
export function MenuDePestanas({ pestanas, activa, onElegir, conError }: { pestanas: Pestana[]; activa: string; onElegir: (id: string) => void; conError: Set<string> }) {
  return (
    <VStack as="nav" role="tablist" aria-orientation="vertical" aria-label="Partes de la ficha" align="stretch" gap={1}
      w={{ base: '180px', md: '220px' }} flexShrink={0} borderRightWidth="1px" borderColor="border" py={3} pr={3}>
      {pestanas.map((p) => {
        const on = p.id === activa;
        return (
          <Button key={p.id} role="tab" aria-selected={on} onClick={() => onElegir(p.id)}
            minH="48px" justifyContent="space-between" variant={on ? 'subtle' : 'ghost'} colorPalette={on ? undefined : 'gray'}
            fontWeight={on ? '600' : 'normal'}>
            <span>{p.label}</span>
            {conError.has(p.id) && <Box as="span" aria-label="tiene un error" w="10px" h="10px" borderRadius="full" bg="red.solid" />}
          </Button>
        );
      })}
    </VStack>
  );
}

// DosColumnas reparte bloques en dos columnas cuando hay ancho, para que la pestaña no obligue a
// deslizar a 1024×600.
export function DosColumnas({ children }: { children: ReactNode }) {
  return (
    <Box css={{ columnCount: 2, columnGap: '40px', '& > *': { breakInside: 'avoid', marginBottom: '20px' }, '@media (max-width: 900px)': { columnCount: 1 } }}>
      {children}
    </Box>
  );
}
