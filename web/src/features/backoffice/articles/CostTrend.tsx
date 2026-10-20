import { Box, Text } from '@chakra-ui/react';
import type { CostHistoryItem } from '../../../api/backoffice';
import { formatUnitCost } from '../../../shared/units';

// La línea de tendencia del costo (spec 042, FR-006, decisión del dueño): las últimas 20 compras y
// correcciones, la más vieja a la izquierda. SVG propio y no una librería: es una sola gráfica y una
// librería pesaría más que la pantalla entera en la tableta (research R5).
//
// El ancho lo pone el contenedor (viewBox + width 100%): de lado y de pie es la misma gráfica. Las
// correcciones van como punto hueco para que no se confundan con lo que costó una compra.

const W = 400;
const H = 140;
const PAD = { l: 58, r: 70, t: 14, b: 24 };

interface Props {
  items: CostHistoryItem[];
  baseUnitCode: string;
  // La fecha en la zona del negocio (useHoraDelNegocio): una fecha sin zona es la del navegador.
  day: (iso: string) => string;
}

export function CostTrend({ items, baseUnitCode, day }: Props) {
  const pts = items.slice(0, 20).slice().reverse();
  if (pts.length === 0) return null;
  const vals = pts.map((p) => Number(p.unitCost));
  const lo = Math.min(...vals) * 0.95;
  const hi = Math.max(...vals) * 1.05 || 1;
  const span = hi - lo || hi;
  const x = (i: number) => PAD.l + (pts.length === 1 ? (W - PAD.l - PAD.r) / 2 : ((W - PAD.l - PAD.r) * i) / (pts.length - 1));
  const y = (v: number) => H - PAD.b - ((H - PAD.t - PAD.b) * (v - lo)) / span;
  const path = pts.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)} ${y(Number(p.unitCost)).toFixed(1)}`).join(' ');
  const last = pts[pts.length - 1];
  const label = (v: number) => formatUnitCost(v, baseUnitCode)?.split(' / ')[0] ?? '';
  return (
    <Box w="100%">
      <svg width="100%" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`Costo en las últimas ${pts.length} compras y correcciones`}>
        <line x1={PAD.l} x2={W - PAD.r} y1={H - PAD.b} y2={H - PAD.b} stroke="var(--chakra-colors-border)" />
        <line x1={PAD.l} x2={W - PAD.r} y1={PAD.t} y2={PAD.t} stroke="var(--chakra-colors-border)" strokeDasharray="3 3" />
        <text x={PAD.l - 6} y={PAD.t + 4} textAnchor="end" fontSize="13" fill="var(--chakra-colors-fg-muted)">{label(hi)}</text>
        <text x={PAD.l - 6} y={H - PAD.b + 4} textAnchor="end" fontSize="13" fill="var(--chakra-colors-fg-muted)">{label(lo)}</text>
        {pts.length > 1 && <path d={path} fill="none" stroke="var(--chakra-colors-brand-solid)" strokeWidth="2" />}
        {pts.map((p, i) => (
          <circle key={`${p.kind}-${p.id}`} cx={x(i)} cy={y(Number(p.unitCost))} r={p.kind === 'correction' ? 4 : 3}
            fill={p.kind === 'correction' ? 'var(--chakra-colors-bg-panel)' : 'var(--chakra-colors-brand-solid)'}
            stroke="var(--chakra-colors-brand-solid)" strokeWidth={p.kind === 'correction' ? 2 : 0} />
        ))}
        <text x={PAD.l} y={H - 4} fontSize="13" fill="var(--chakra-colors-fg-muted)">{day(pts[0].at)}</text>
        <text x={W - PAD.r} y={H - 4} textAnchor="end" fontSize="13" fill="var(--chakra-colors-fg-muted)">{day(last.at)}</text>
        <text x={W - PAD.r + 6} y={y(Number(last.unitCost)) + 5} fontSize="14" fontWeight="700" fill="var(--chakra-colors-fg)">{label(Number(last.unitCost))}</text>
      </svg>
      {pts.length === 1 && <Text fontSize="xs" color="fg.muted">Una sola compra todavía: la tendencia sale con la segunda.</Text>}
    </Box>
  );
}
