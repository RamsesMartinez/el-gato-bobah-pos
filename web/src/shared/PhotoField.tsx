import { useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Box, Button, HStack, IconButton, Text, VStack } from '@chakra-ui/react';
import { LuCamera, LuCrop, LuMove, LuRefreshCw, LuTrash2, LuZoomIn, LuZoomOut } from 'react-icons/lu';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter, DialogTitle, DialogCloseTrigger,
} from '../components/ui/dialog';
import { ConfirmSheet } from '../components/ConfirmSheet';
import { CatalogImage } from '../components/CatalogImage';
import { adminApi } from '../api/admin';
import { ApiError } from '../api/client';
import {
  PhotoError, SMALL_SIDE, canPreparePhotos, cropFor, photoSize, preparePhoto, toBase64, type Framing,
} from '../media/preparePhoto';

interface Props {
  kind: 'products' | 'categories';
  id: number;
  name: string;
  imageUrl: string | null | undefined;
  // Lo que se ve sin foto propia: la de su categoría (solo productos) o el dibujo de su familia.
  categoryImageUrl?: string | null;
  family?: string | null;
  size?: number;
  onChanged: (url: string | null) => void;
}

// Lo que quien opera puede hacer con el error: el rechazo del servidor (medidas, peso) se dice tal
// cual; lo demás (sin red, el almacenamiento caído) con una instrucción, sin el texto del error.
function saveMessage(e: unknown, what: 'guardar' | 'quitar'): string {
  if (e instanceof PhotoError) return e.message;
  if (e instanceof ApiError && e.status < 500 && e.status !== 0) return e.message;
  return `No se pudo ${what} la foto. Revisa la conexión e intenta de nuevo.`;
}

// PhotoField pone, cambia y quita la foto de un producto o una categoría (spec 091, maquetas M1 y M2).
// La foto se guarda al tocar «Guardar foto», aparte del «Guardar» del formulario, como los grupos.
export function PhotoField({ kind, id, name, imageUrl, categoryImageUrl, family, size = 132, onChanged }: Props) {
  const input = useRef<HTMLInputElement>(null);
  const actions = useRef<HTMLDivElement>(null);
  const [supported] = useState(canPreparePhotos);
  const [pending, setPending] = useState<{ file: File; url: string; width: number; height: number } | null>(null);
  const [error, setError] = useState('');
  const [cropError, setCropError] = useState('');
  const [saved, setSaved] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  // La foto vigente es la última elegida: leer una grande tarda, y la que llega después no puede
  // pisar a la que se eligió después. Al desmontar, nada que llegue tarde abre el recorte.
  const choice = useRef(0);
  useEffect(() => () => { choice.current = -1; }, []);

  useEffect(() => () => { if (pending) URL.revokeObjectURL(pending.url); }, [pending]);

  // Tras guardar o quitar, el botón que se tocó desaparece (cambia «Agregar» por «Cambiar» y al
  // revés); el foco va al que quedó y no se pierde en la página.
  const refocus = () => requestAnimationFrame(() => actions.current?.querySelector<HTMLButtonElement>('button')?.focus());

  const choose = async (f: File | undefined) => {
    if (input.current) input.current.value = ''; // el mismo archivo otra vez también dispara el cambio
    if (!f) return;
    const mine = ++choice.current;
    setError('');
    setSaved(false);
    try {
      const { width, height } = await photoSize(f);
      if (choice.current !== mine) return;
      setCropError('');
      setPending({ file: f, url: URL.createObjectURL(f), width, height });
    } catch (e) {
      if (choice.current !== mine) return;
      setError(e instanceof PhotoError ? e.message : 'Esta foto no se puede abrir. Elige una en JPG o PNG.');
    }
  };

  const save = async (framing: Framing) => {
    if (!pending) return;
    setBusy(true);
    setCropError('');
    try {
      const { large, small } = await preparePhoto(pending.file, framing);
      const [image1200, image400] = await Promise.all([toBase64(large), toBase64(small)]);
      const res = await adminApi.setImage(kind, id, { image1200, image400 });
      onChanged(res.imageUrl);
      setPending(null);
      setSaved(true);
      refocus();
    } catch (e) {
      // El aviso va dentro del recorte: afuera quedaría tapado por el diálogo, y la foto sigue ahí
      // para reintentar.
      setCropError(saveMessage(e, 'guardar'));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    setError('');
    try {
      await adminApi.removeImage(kind, id);
      onChanged(null);
      setSaved(false);
      setConfirming(false);
      refocus();
    } catch (e) {
      setConfirming(false);
      setError(saveMessage(e, 'quitar'));
    } finally {
      setBusy(false);
    }
  };

  const note = !supported ? 'Este navegador no puede preparar la foto. Ábrelo en Chrome o Edge.' : error || (saved ? 'Foto guardada' : '');

  return (
    <VStack w={`${size}px`} flexShrink={0} gap={2} align="stretch">
      <Box position="relative">
        <CatalogImage imageUrl={imageUrl} categoryImageUrl={categoryImageUrl} family={family} size={size} alt={name} />
        {!imageUrl && (
          <Badge position="absolute" bottom={1} left={1} size="sm" variant="solid" colorPalette="gray">Sin foto</Badge>
        )}
      </Box>
      {/* Invisible y no display:none: Safari táctil no abre el selector de un input oculto así. */}
      <input ref={input} type="file" accept="image/jpeg,image/png,image/webp" tabIndex={-1} aria-hidden="true"
        style={{ position: 'absolute', width: 1, height: 1, opacity: 0, pointerEvents: 'none' }}
        onChange={(e) => void choose(e.target.files?.[0])} />
      <Box ref={actions}>
        {imageUrl ? (
          <HStack gap={1}>
            <IconButton aria-label="Cambiar foto" title="Cambiar foto" variant="outline" minH="44px" flex="1"
              disabled={!supported || busy} onClick={() => input.current?.click()}><LuRefreshCw /></IconButton>
            <IconButton aria-label="Quitar foto" title="Quitar foto" variant="outline" colorPalette="red" minH="44px" flex="1"
              disabled={busy} onClick={() => setConfirming(true)}><LuTrash2 /></IconButton>
          </HStack>
        ) : (
          <Button minH="44px" size="sm" w="100%" disabled={!supported || busy} onClick={() => input.current?.click()}>
            <LuCamera /> Agregar foto
          </Button>
        )}
      </Box>
      {/* Solo cuando hay algo que decir: el renglón vacío le quitaría alto al formulario. */}
      {note && (
        <Text fontSize="xs" color={error || !supported ? 'fg.error' : 'fg.muted'} role={error ? 'alert' : 'status'}>{note}</Text>
      )}

      {pending && (
        <CropDialog src={pending.url} width={pending.width} height={pending.height} busy={busy} error={cropError}
          onCancel={() => setPending(null)} onSave={(f) => void save(f)} />
      )}
      <ConfirmSheet isOpen={confirming} destructive title="¿Quitar la foto?" confirmLabel="Quitar" loading={busy}
        description={kind === 'categories'
          ? 'Los productos sin foto propia dejarán de mostrar ésta.'
          : 'Se verá la foto de su categoría o su dibujo.'}
        onConfirm={() => void remove()} onCancel={() => setConfirming(false)} />
    </VStack>
  );
}

// CropDialog (maqueta M2, opción B): la foto centrada tal como se guardará; ajustar el encuadre es
// opcional y cuesta un toque más.
function CropDialog({ src, width, height, busy, error, onCancel, onSave }: {
  src: string; width: number; height: number; busy: boolean; error: string; onCancel: () => void; onSave: (f: Framing) => void;
}) {
  const [adjusting, setAdjusting] = useState(false);
  const [framing, setFraming] = useState<Framing>({ zoom: 1, cx: 0.5, cy: 0.5 });
  const frame = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; y: number; f: Framing } | null>(null);
  const crop = useMemo(() => cropFor(width, height, framing), [width, height, framing]);
  const view = adjusting ? 'min(340px, 44dvh)' : 'min(340px, 52dvh)';
  // Acercar más allá de esto deja un cuadrado de menos de 400 px de la foto original: borroso.
  const maxZoom = Math.max(1, Math.min(width, height) / SMALL_SIDE);

  const zoomBy = (d: number) => setFraming((f) => ({ ...f, zoom: Math.min(maxZoom, Math.max(1, f.zoom + d)) }));
  const onPointerMove = (e: React.PointerEvent) => {
    if (!drag.current || !frame.current) return;
    const px = frame.current.clientWidth || 1;
    const scale = crop.size / px; // pixeles de la foto por pixel de pantalla
    const { x, y, f } = drag.current;
    // Arrastrar la foto a la derecha mueve el encuadre a la izquierda; cropFor la mantiene dentro.
    const cx = f.cx - ((e.clientX - x) * scale) / width;
    const cy = f.cy - ((e.clientY - y) * scale) / height;
    const c = cropFor(width, height, { ...f, cx, cy });
    setFraming({ ...f, cx: (c.x + c.size / 2) / width, cy: (c.y + c.size / 2) / height });
  };

  // La foto se dibuja escalada para que el cuadrado elegido llene el marco.
  const pct = (v: number) => `${(v / crop.size) * 100}%`;
  return (
    // Mientras se guarda no se cierra: cancelar a media subida no la detendría.
    <DialogRoot open onOpenChange={(e) => { if (!e.open && !busy) onCancel(); }} placement="center" size="md" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader py={3}><DialogTitle>{adjusting ? 'Acomoda la foto' : 'Así se verá'}</DialogTitle></DialogHeader>
        <DialogCloseTrigger disabled={busy} />
        <DialogBody py={2}>
          <VStack gap={3}>
            <Box ref={frame} position="relative" w={view} h={view} borderRadius="lg" overflow="hidden" bg="gray.900" flexShrink={0}
              css={{ touchAction: 'none', cursor: adjusting ? 'grab' : 'default' }}
              onPointerDown={(e) => { if (adjusting) { drag.current = { x: e.clientX, y: e.clientY, f: framing }; e.currentTarget.setPointerCapture?.(e.pointerId); } }}
              onPointerMove={onPointerMove}
              onPointerUp={() => { drag.current = null; }} onPointerCancel={() => { drag.current = null; }}>
              <img src={src} alt="" draggable={false} style={{
                position: 'absolute', maxWidth: 'none', left: pct(-crop.x), top: pct(-crop.y),
                width: pct(width), height: pct(height), userSelect: 'none',
              }} />
              {adjusting && <Box position="absolute" inset={0} pointerEvents="none" css={{
                backgroundImage: 'linear-gradient(#fff6 1px, transparent 1px), linear-gradient(90deg, #fff6 1px, transparent 1px)',
                backgroundSize: '33.33% 33.33%', outline: '2px solid white', outlineOffset: '-2px',
              }} />}
            </Box>
            {adjusting ? (
              <>
                <HStack gap={3}>
                  <IconButton aria-label="Alejar" variant="outline" minW="44px" minH="44px" disabled={framing.zoom <= 1}
                    onClick={() => zoomBy(-0.25)}><LuZoomOut /></IconButton>
                  <Text minW="56px" textAlign="center" fontVariantNumeric="tabular-nums">{Math.round(framing.zoom * 100)}%</Text>
                  <IconButton aria-label="Acercar" variant="outline" minW="44px" minH="44px" disabled={framing.zoom >= maxZoom}
                    onClick={() => zoomBy(0.25)}><LuZoomIn /></IconButton>
                </HStack>
                <HStack fontSize="sm" color="fg.muted" gap={1}><LuMove /><Text>Arrastra para acomodar</Text></HStack>
              </>
            ) : (
              <Button variant="outline" minH="44px" onClick={() => setAdjusting(true)}><LuCrop /> Ajustar encuadre</Button>
            )}
            {error && <Text fontSize="sm" color="fg.error" role="alert" textAlign="center">{error}</Text>}
          </VStack>
        </DialogBody>
        <DialogFooter py={3}>
          <Button variant="ghost" minH="44px" mr={3} disabled={busy} onClick={onCancel}>Cancelar</Button>
          <Button minH="44px" loading={busy} onClick={() => onSave(framing)}>Guardar foto</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
