import { useEffect, useId, useRef, useState } from 'react';
import { Box, Text } from '@chakra-ui/react';
import type { CostHistoryItem } from '../../../api/backoffice';
import { formatUnitCost } from '../../../shared/units';
import { useContainerWidth } from '../../../hooks/useContainerWidth';

// La línea de tendencia del costo (spec 042, FR-006; rediseño en la 044, maqueta «grafica.html»): las
// últimas 20 compras y correcciones, la más vieja a la izquierda. SVG propio y no una librería: es una
// sola gráfica y una librería pesaría más que la pantalla entera en la tableta (research R5).
//
// El viewBox mide lo mismo que el contenedor: una unidad es un píxel y los textos miden lo que dicen
// (con un viewBox fijo de 440, en la ficha de 368 px los de 11 quedaban en 9 px). Las correcciones van como punto hueco para que
// no se confundan con lo que costó una compra. Los colores son tokens del tema: en oscuro se invierten.

// Ancho de respaldo mientras no hay medida (jsdom).
const FALLBACK_W = 440;
const H = 190;
const PAD = { t: 18, b: 30 };
// Ningún texto de la gráfica baja de 13 px: el piso de lectura en la tableta.
const FS = 13;
const BUBBLE_FS = 14;
// ~Ancho de un carácter de Inter (dígitos tabulares), en em; en negrita un poco más.
const EM = 0.6;
const EM_BOLD = 0.62;
const C = {
  brand: 'var(--chakra-colors-brand-solid)',
  line: 'var(--chakra-colors-border)',
  muted: 'var(--chakra-colors-fg-muted)',
  fg: 'var(--chakra-colors-fg)',
  inverse: 'var(--chakra-colors-bg)',
  panel: 'var(--chakra-colors-bg-panel)',
};

interface Props {
  items: CostHistoryItem[];
  baseUnitCode: string;
  // La fecha en la zona del negocio (useHoraDelNegocio): una fecha sin zona es la del navegador.
  day: (iso: string) => string;
}

// smooth traza una curva Catmull-Rom que pasa por cada punto: suaviza sin inventar picos.
function smooth(p: [number, number][]) {
  if (p.length < 3) return p.map((q, i) => `${i ? 'L' : 'M'}${q[0].toFixed(1)} ${q[1].toFixed(1)}`).join(' ');
  let d = `M${p[0][0].toFixed(1)} ${p[0][1].toFixed(1)}`;
  for (let i = 0; i < p.length - 1; i++) {
    const p0 = p[i - 1] ?? p[i], p1 = p[i], p2 = p[i + 1], p3 = p[i + 2] ?? p2;
    const c1 = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6];
    const c2 = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6];
    d += ` C${c1[0].toFixed(1)} ${c1[1].toFixed(1)},${c2[0].toFixed(1)} ${c2[1].toFixed(1)},${p2[0].toFixed(1)} ${p2[1].toFixed(1)}`;
  }
  return d;
}

export function CostTrend({ items, baseUnitCode, day }: Props) {
  const gradId = useId();
  const svgRef = useRef<SVGSVGElement>(null);
  const [picked, setPicked] = useState<number | null>(null);
  const { ref: boxRef, width } = useContainerWidth<HTMLDivElement>();
  const W = width > 0 ? Math.round(width) : FALLBACK_W;

  // Tocar fuera de la gráfica cierra la etiqueta.
  useEffect(() => {
    if (picked === null) return;
    const off = (e: PointerEvent) => { if (!svgRef.current?.contains(e.target as Node)) setPicked(null); };
    document.addEventListener('pointerdown', off);
    return () => document.removeEventListener('pointerdown', off);
  }, [picked]);

  const pts = items.slice(0, 20).slice().reverse();
  if (pts.length === 0) return null;
  const vals = pts.map((p) => Number(p.unitCost));
  const min = Math.min(...vals);
  const max = Math.max(...vals);
  // Margen abajo de un cuarto del rango: el mínimo pegado al piso se lee como si valiera casi cero.
  const money = (v: number) => formatUnitCost(v, baseUnitCode)?.split(' / ')[0] ?? '';
  const lastText = money(vals[vals.length - 1]);
  // La burbuja crece con la cifra («$1,250.00» no cabía en 64) y el margen derecho con ella.
  const bubbleW = Math.ceil(lastText.length * EM_BOLD * BUBBLE_FS + 18);
  const axisW = Math.max(money(max).length, money(min).length) * EM * FS;
  const PADL = Math.ceil(axisW + 10);
  const PADR = bubbleW + 14;
  const range = max - min || max * 0.1 || 1;
  const lo = min - range * 0.25;
  const hi = max + range * 0.1;
  const span = hi - lo;
  const x = (i: number) => PADL + (pts.length === 1 ? (W - PADL - PADR) / 2 : ((W - PADL - PADR) * i) / (pts.length - 1));
  const y = (v: number) => PAD.t + (H - PAD.t - PAD.b) * (1 - (v - lo) / span);
  const P = pts.map((p, i) => [x(i), y(Number(p.unitCost))] as [number, number]);
  const last = pts.length - 1;
  const [lx, ly] = P[last];
  const base = H - PAD.b;
  const curve = smooth(P);

  // Con 20 puntos no caben zonas de 44 px por punto: el toque elige el más cercano por x.
  const pick = (e: React.PointerEvent<SVGSVGElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    const vx = r.width ? ((e.clientX - r.left) * W) / r.width : e.clientX;
    let best = 0;
    P.forEach((q, i) => { if (Math.abs(q[0] - vx) < Math.abs(P[best][0] - vx)) best = i; });
    setPicked(best);
  };

  const tip = picked === null ? null : (() => {
    const p = pts[picked];
    const text = `${day(p.at)} · ${formatUnitCost(Number(p.unitCost), baseUnitCode) ?? ''}${p.kind === 'correction' ? ' · a mano' : ''}`;
    const w = Math.min(W - 8, text.length * EM * FS + 18);
    const [px, py] = P[picked];
    // Arriba del punto si cabe, si no debajo; y sin salirse por los lados.
    const ty = py - 40 >= 2 ? py - 40 : py + 12;
    const tx = Math.max(4, Math.min(W - w - 4, px - w / 2));
    return (
      <g data-tip="" pointerEvents="none">
        <line x1={px} x2={px} y1={PAD.t} y2={base} stroke={C.muted} strokeDasharray="2 3" />
        <rect x={tx} y={ty} width={w} height={28} rx={8} fill={C.fg} />
        <text x={tx + w / 2} y={ty + 19} textAnchor="middle" fontSize={FS} fontWeight="600" fill={C.inverse}>{text}</text>
      </g>
    );
  })();

  return (
    <Box w="100%" ref={boxRef}>
      <svg ref={svgRef} width="100%" viewBox={`0 0 ${W} ${H}`} role="img" onPointerDown={pick} style={{ cursor: 'pointer', touchAction: 'manipulation' }}
        aria-label={`Costo en las últimas ${pts.length} compras y correcciones`}>
        <defs>
          <linearGradient id={gradId} x1="0" x2="0" y1="0" y2="1">
            <stop offset="0" stopColor={C.brand} stopOpacity=".22" />
            <stop offset="1" stopColor={C.brand} stopOpacity="0" />
          </linearGradient>
        </defs>
        {/* Guías en el mínimo y el máximo, con su cifra: sin ellas la altura no dice cuánto costó. */}
        {(min === max ? [['max', max]] as const : [['max', max], ['min', min]] as const).map(([k, v]) => (
          <g key={k}>
            <line x1={PADL} x2={W - PADR} y1={y(v)} y2={y(v)} stroke={C.line} strokeDasharray="2 4" />
            {pts.length > 1 && <text data-axis={k} x={PADL - 6} y={y(v) + 4} textAnchor="end" fontSize={FS} fill={C.muted}>{money(v)}</text>}
          </g>
        ))}
        {pts.length > 1 && (
          <>
            <path data-area="" d={`${curve} L${lx.toFixed(1)} ${base} L${P[0][0].toFixed(1)} ${base}Z`} fill={`url(#${gradId})`} />
            <path data-line="" d={curve} fill="none" stroke={C.brand} strokeWidth="2.5" strokeLinecap="round" />
          </>
        )}
        <line x1={PADL} x2={W - PADR} y1={base} y2={base} stroke={C.line} />
        {pts.map((p, i) => p.kind === 'correction'
          ? <circle key={`${p.kind}-${p.id}`} data-point="correction" cx={P[i][0]} cy={P[i][1]} r={4.5} fill={C.panel} stroke={C.brand} strokeWidth={2} />
          : i === last || i === picked
            ? <g key={`${p.kind}-${p.id}`} data-point="purchase"><circle cx={P[i][0]} cy={P[i][1]} r={8} fill={C.brand} opacity={0.18} /><circle cx={P[i][0]} cy={P[i][1]} r={4.5} fill={C.brand} /></g>
            : <circle key={`${p.kind}-${p.id}`} data-point="purchase" cx={P[i][0]} cy={P[i][1]} r={2.5} fill={C.brand} />)}
        <rect x={lx + 10} y={ly - 14} width={bubbleW} height={28} rx={14} fill={C.fg} />
        <text x={lx + 10 + bubbleW / 2} y={ly + 5} textAnchor="middle" fontSize={BUBBLE_FS} fontWeight="700" fill={C.inverse}>{lastText}</text>
        {pts.length > 1 && <text x={PADL} y={H - 8} fontSize={FS} fill={C.muted}>{day(pts[0].at)}</text>}
        <text x={W - PADR} y={H - 8} textAnchor="end" fontSize={FS} fill={C.muted}>{day(pts[last].at)}</text>
        {tip}
      </svg>
      {pts.length === 1 && <Text fontSize="xs" color="fg.muted">Un solo registro todavía: la tendencia sale con el segundo.</Text>}
    </Box>
  );
}
