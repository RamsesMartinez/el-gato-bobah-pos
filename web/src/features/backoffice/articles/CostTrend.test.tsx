import { afterEach, describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Provider } from '../../../components/ui/provider';
import { CostTrend } from './CostTrend';
import type { CostHistoryItem } from '../../../api/backoffice';

const item = (id: number, cost: string, kind: 'purchase' | 'correction' = 'purchase') => ({
  kind, at: `2026-10-${String(id).padStart(2, '0')}`, id, unitCost: cost, presentation: null, packQty: null, packCost: null, supplier: null, user: null,
}) as unknown as CostHistoryItem;

function montar(items: CostHistoryItem[]) {
  return render(<Provider><CostTrend items={items} baseUnitCode="g" day={(s) => s.slice(5)} /></Provider>);
}
// jsdom no mide: el SVG ocupa su viewBox (440 de ancho) a partir de x = 0.
function medir(svg: Element) {
  svg.getBoundingClientRect = () => ({ left: 0, top: 0, width: 440, height: 190, right: 440, bottom: 190, x: 0, y: 0, toJSON: () => ({}) });
}

// E9: la gráfica con 0, 1 o varios puntos, y tocar para leer un punto.
describe('Gráfica de costos', () => {
  it('sin registros no dibuja nada', () => {
    const { container } = montar([]);
    expect(container.querySelector('svg')).toBeNull();
  });

  it('con un registro pinta su punto y su cifra, sin línea ni área', () => {
    const { container } = montar([item(19, '0.022', 'correction')]);
    expect(screen.getByText('$22.00')).toBeInTheDocument();
    expect(container.querySelector('[data-line]')).toBeNull();
    expect(container.querySelector('[data-area]')).toBeNull();
    expect(screen.getByText(/Un solo registro/)).toBeInTheDocument();
  });

  it('con varios pinta línea, área, el último costo en etiqueta y las correcciones huecas', () => {
    // El servidor manda la más reciente primero.
    const { container } = montar([item(19, '0.060', 'correction'), item(12, '0.070'), item(5, '0.057')]);
    expect(container.querySelector('[data-line]')).not.toBeNull();
    expect(container.querySelector('[data-area]')).not.toBeNull();
    expect(screen.getByText('$60.00')).toBeInTheDocument();
    expect(container.querySelectorAll('[data-point="correction"]').length).toBe(1);
    expect(container.querySelector('[data-point="correction"]')?.getAttribute('fill')).toContain('bg-panel');
    expect(screen.getByText('10-05')).toBeInTheDocument();
    expect(screen.getByText('10-19')).toBeInTheDocument();
  });

  it('tocar elige el punto más cercano por x y muestra su fecha y costo; tocar fuera lo cierra', () => {
    const { container } = montar([item(19, '0.060', 'correction'), item(12, '0.070'), item(5, '0.057')]);
    const svg = container.querySelector('svg')!;
    medir(svg);
    // El primer punto está a la izquierda; un toque cerca de él, aunque no encima, lo elige.
    fireEvent.pointerDown(svg, { clientX: 40, clientY: 5 });
    const tip = container.querySelector('[data-tip]');
    expect(tip?.textContent).toContain('10-05');
    expect(tip?.textContent).toContain('$57.00');
    expect(svg.contains(tip)).toBe(true);
    fireEvent.pointerDown(svg, { clientX: 200, clientY: 5 });
    expect(container.querySelector('[data-tip]')?.textContent).toContain('$70.00');
    fireEvent.pointerDown(document.body);
    expect(container.querySelector('[data-tip]')).toBeNull();
  });
  it('rotula el costo mínimo y el máximo, y el mínimo no toca el piso', () => {
    const { container } = montar([item(19, '0.060'), item(12, '0.070'), item(5, '0.057')]);
    expect(container.querySelector('[data-axis="max"]')?.textContent).toBe('$70.00');
    expect(container.querySelector('[data-axis="min"]')?.textContent).toBe('$57.00');
    const ys = [...container.querySelectorAll('[data-point]')].map((g) => Number((g.tagName === 'circle' ? g : g.querySelector('circle'))!.getAttribute('cy')));
    // El piso está en 160 (190 - 30): el punto más bajo queda al menos 20 px arriba.
    expect(Math.max(...ys)).toBeLessThanOrEqual(140);
  });

  // Spec 044: la ficha mide 480 px de ancho en 1024 y 368 px en 800. Con el viewBox fijo de 440 la
  // gráfica se encogía y sus textos de 11 px quedaban en 9 px reales.
  afterEach(() => vi.restoreAllMocks());
  const ancho = (w: number) => vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(w);

  it.each([368, 480])('a %i px de ancho, ningún texto mide menos de 13 px reales', (w) => {
    ancho(w);
    const { container } = montar([item(19, '0.060'), item(12, '0.070'), item(5, '0.057')]);
    const svg = container.querySelector('svg')!;
    const vbW = Number(svg.getAttribute('viewBox')!.split(' ')[2]);
    const sizes = [...svg.querySelectorAll('text')].map((t) => (Number(t.getAttribute('font-size')) * w) / vbW);
    expect(sizes.length).toBeGreaterThan(0);
    expect(Math.min(...sizes)).toBeGreaterThanOrEqual(13);
  });

  it('la burbuja del último costo crece con la cifra y no se sale de la gráfica', () => {
    ancho(368);
    const { container } = montar([item(19, '1.25'), item(12, '0.070')]);
    const etiqueta = screen.getAllByText('$1,250.00').find((t) => t.getAttribute('font-weight') === '700')!;
    const burbuja = etiqueta.previousElementSibling!;
    const vbW = Number(container.querySelector('svg')!.getAttribute('viewBox')!.split(' ')[2]);
    const w = Number(burbuja.getAttribute('width'));
    // Inter en negrita: ~0.62 em por dígito, más el relleno de los dos lados.
    expect(w).toBeGreaterThanOrEqual('$1,250.00'.length * 0.62 * Number(etiqueta.getAttribute('font-size')) + 16);
    expect(Number(burbuja.getAttribute('x')) + w).toBeLessThanOrEqual(vbW);
  });
});
