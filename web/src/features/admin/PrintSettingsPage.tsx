import { useMemo, useRef, useState } from 'react';
import { Box, Button, Center, Heading, HStack, IconButton, Image, Input, Spinner, Text, Textarea, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LuCircleHelp, LuCopy, LuImageUp, LuPrinter, LuReceipt, LuTrash2 } from 'react-icons/lu';

import { posApi, type BusinessSettings, type TicketSettingsInput } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { Switch } from '../../components/ui/switch';
import { Page } from '../../components/Page';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody } from '../../components/ui/dialog';
import { useTicketBusinessInfo } from '../tickets/ticketBusinessInfo';
import { TicketPreview } from '../tickets/TicketPreview';
import { sampleTicketOrder } from '../../utils/printReceipt';

// Lo que sale impreso en el ticket y cómo se dispara la impresión. El backend es la autoridad
// (el PUT exige rol y valida los largos); esta pantalla es la UX para editarlo.
export function PrintSettingsPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const fileInput = useRef<HTMLInputElement>(null);
  // La vista previa usa el MISMO data URI que va al ticket: un <img src="/api/..."> no llevaría el
  // header de Authorization y saldría roto, además de poder diferir de lo que se imprime.
  const { data: ticketInfo } = useTicketBusinessInfo();

  // null = sin edición local todavía: el input refleja lo cargado sin un useEffect que sincronice.
  const [draft, setDraft] = useState<TicketSettingsInput | null>(null);
  const [help, setHelp] = useState(false);
  const [sample, setSample] = useState(false);
  // El pedido de muestra es fijo: se arma una vez y no cambia entre renders.
  const sampleOrder = useMemo(() => sampleTicketOrder(), []);
  // El destino del acceso directo se arma con el origen real del sistema: la misma instrucción
  // sirve en cualquier negocio y en cualquier dominio, sin que nadie tenga que editarla.
  const kioskTarget = `msedge.exe --kiosk-printing ${window.location.origin}`;
  const copyKioskTarget = () => {
    void navigator.clipboard?.writeText(kioskTarget).then(
      () => toaster.create({ title: 'Copiado', type: 'success' }),
      () => toaster.create({ title: 'No se pudo copiar', description: 'Selecciónalo y cópialo a mano.', type: 'error' }),
    );
  };
  const field = <K extends keyof TicketSettingsInput>(k: K): string =>
    String(draft?.[k] ?? (data?.[k as keyof BusinessSettings] as string | undefined) ?? '');
  const set = (patch: TicketSettingsInput) => setDraft((d) => ({ ...d, ...patch }));

  const applied = (bs: BusinessSettings) => {
    qc.setQueryData(['business-settings'], bs);
    qc.invalidateQueries({ queryKey: ['business-settings'] });
  };

  const save = useMutation({
    mutationFn: () =>
      posApi.updateTicketSettings({
        businessName: field('businessName'),
        address: field('address'),
        phone: field('phone'),
        headerNote: field('headerNote'),
        footerNote: field('footerNote'),
      }),
    onSuccess: (bs) => {
      applied(bs);
      setDraft(null);
      toaster.create({ title: 'Ticket actualizado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo guardar', description: String(e), type: 'error' }),
  });

  // El interruptor guarda solo: no tiene sentido pedir "Guardar" para un sí/no.
  const setAutoPrint = useMutation({
    mutationFn: (v: boolean) => posApi.updateTicketSettings({ autoPrintOnClose: v }),
    onSuccess: applied,
    onError: (e) => toaster.create({ title: 'No se pudo cambiar', description: String(e), type: 'error' }),
  });

  const uploadLogo = useMutation({
    mutationFn: (file: File) => posApi.uploadTicketLogo(file),
    onSuccess: (bs) => {
      applied(bs);
      // La clave del logo lleva logoUpdatedAt, así que el ticket toma la imagen nueva sola.
      qc.invalidateQueries({ queryKey: ['ticket-logo'] });
      toaster.create({ title: 'Logo actualizado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo subir', description: String(e), type: 'error' }),
  });

  const removeLogo = useMutation({
    mutationFn: () => posApi.deleteTicketLogo(),
    onSuccess: (bs) => {
      applied(bs);
      qc.invalidateQueries({ queryKey: ['ticket-logo'] });
      toaster.create({ title: 'Se restauró el logo por defecto', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo quitar', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page maxW="620px">
      <Heading size="lg" mb={1}>Impresión</Heading>
      <Text color="fg.muted" mb={6}>Qué sale en el ticket de venta y cuándo se imprime.</Text>

      <Box mb={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Logo del ticket</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          PNG o JPEG, hasta 256 KB. La impresora es en blanco y negro: los logos de trazo grueso se
          ven bien; los que tienen degradados o mucho detalle salen manchados.
        </Text>
        <HStack gap={4} align="center">
          <Box borderWidth="1px" borderColor="border" borderRadius="md" p={2} bg="white">
            <Image
              src={ticketInfo?.logoDataUri}
              alt="Logo del ticket" maxH="72px" maxW="120px" objectFit="contain"
            />
          </Box>
          <VStack align="stretch" gap={2}>
            <input
              ref={fileInput} type="file" accept="image/png,image/jpeg" hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) uploadLogo.mutate(f);
                e.target.value = ''; // permite volver a elegir el mismo archivo
              }}
            />
            <Button size="sm" loading={uploadLogo.isPending} onClick={() => fileInput.current?.click()}>
              <LuImageUp /> Subir logo
            </Button>
            <Button size="sm" variant="outline" disabled={!data?.hasLogo} loading={removeLogo.isPending}
              onClick={() => removeLogo.mutate()}>
              <LuTrash2 /> Usar el de siempre
            </Button>
          </VStack>
        </HStack>
      </Box>

      <Box mb={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Encabezado y pie</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          Cada renglón del ticket cabe en unos 32 caracteres. Puedes usar varias líneas.
        </Text>
        <VStack align="stretch" gap={3}>
          <Input placeholder="Nombre del negocio" maxLength={60} value={field('businessName')}
            onChange={(e) => set({ businessName: e.target.value })} />
          <Input placeholder="Dirección (opcional)" maxLength={120} value={field('address')}
            onChange={(e) => set({ address: e.target.value })} />
          <Input placeholder="Teléfono (opcional)" maxLength={30} value={field('phone')}
            onChange={(e) => set({ phone: e.target.value })} />
          <Textarea placeholder="Texto superior: va arriba del detalle (opcional)" rows={3} maxLength={400}
            value={field("headerNote")} onChange={(e) => set({ headerNote: e.target.value })} />
          <Textarea placeholder="Texto inferior: cierra el ticket (opcional)" rows={8} maxLength={400}
            value={field("footerNote")} onChange={(e) => set({ footerNote: e.target.value })} />
          <HStack gap={2}>
            <Button loading={save.isPending} disabled={!field('businessName').trim()}
              onClick={() => save.mutate()}>Guardar</Button>
            <Button variant="outline" onClick={() => setSample(true)}>
              <LuReceipt /> Ticket de prueba
            </Button>
          </HStack>
        </VStack>
      </Box>

      <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <HStack justify="space-between" align="start" gap={4}>
          <Box>
            <HStack gap={1} mb={1}>
              <LuPrinter />
              <Text fontWeight="700">Imprimir al cerrar la venta</Text>
              <IconButton aria-label="Cómo configurar la impresión directa" size="2xs" variant="ghost"
                onClick={() => setHelp(true)}><LuCircleHelp /></IconButton>
            </HStack>
            <Text fontSize="sm" color="fg.muted">
              El ticket sale solo al cobrar. Requiere la impresora configurada para imprimir directo.
            </Text>
          </Box>
          <Switch
            checked={data?.autoPrintOnClose ?? false}
            disabled={setAutoPrint.isPending}
            onCheckedChange={(e) => setAutoPrint.mutate(e.checked)}
          />
        </HStack>
      </Box>

      {/* Ticket de prueba: se ve antes de imprimir, y sale marcado para que no se confunda con
          una venta si acaba en manos de un cliente. */}
      <TicketPreview order={sampleOrder} sample isOpen={sample} onClose={() => setSample(false)} />

      <DialogRoot open={help} onOpenChange={(e) => setHelp(e.open)} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent mx={4} borderRadius="2xl">
          <DialogBody py={6}>
            <Text fontWeight="700" fontSize="lg" mb={1}>Imprimir sin preguntar</Text>
            <Text fontSize="sm" color="fg.muted" mb={5}>
              Se configura una vez en cada equipo. Toma unos 2 minutos.
            </Text>
            <VStack align="stretch" gap={4}>
              <KioskStep n={1} title="Deja la impresora de tickets como predeterminada">
                Abre <b>Configuración → Bluetooth y dispositivos → Impresoras y escáneres</b>, entra a
                tu impresora de tickets y elige <b>Establecer como predeterminada</b>.
              </KioskStep>

              <KioskStep n={2} title="Crea un acceso directo al sistema">
                Clic derecho en el escritorio → <b>Nuevo → Acceso directo</b>. Cuando pida la
                ubicación, pega esto:
                <Box mt={2} bg="bg.muted" borderRadius="md" p={3} fontSize="xs" fontFamily="mono" overflowX="auto">
                  {kioskTarget}
                </Box>
                <Button mt={2} size="xs" variant="outline" onClick={copyKioskTarget}>
                  <LuCopy /> Copiar
                </Button>
              </KioskStep>

              <KioskStep n={3} title="Ponle nombre y ábrelo desde ahí">
                Llámalo como quieras (por ejemplo, el nombre de tu negocio). De ahí en adelante, el
                equipo debe abrir el sistema <b>siempre</b> con ese acceso directo.
              </KioskStep>
            </VStack>

            <Box mt={5} borderWidth="1px" borderColor="border" borderRadius="md" p={3}>
              <Text fontSize="sm">
                <b>¿Cómo sé que funcionó?</b> Cobra una venta: el ticket sale solo. Si aparece un
                cuadro pidiendo confirmación, el sistema se abrió con el acceso directo de siempre y
                no con el nuevo.
              </Text>
            </Box>

            <Button mt={5} w="100%" onClick={() => setHelp(false)}>Entendido</Button>
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </Page>
  );
}

// KioskStep es un paso numerado de la guía. Vive aquí y no en components/ porque es lo único que lo
// usa; se saca cuando haya un segundo instructivo (impresora de cocina, fase 2).
function KioskStep({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <HStack align="start" gap={3}>
      <Center minW="28px" h="28px" borderRadius="full" bg="colorPalette.solid" color="colorPalette.contrast"
        fontWeight="800" fontSize="sm">{n}</Center>
      <Box flex="1">
        <Text fontWeight="700" fontSize="sm" mb={1}>{title}</Text>
        <Box fontSize="sm" color="fg.muted">{children}</Box>
      </Box>
    </HStack>
  );
}
