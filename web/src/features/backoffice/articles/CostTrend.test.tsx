import { describe, it, expect } from 'vitest';
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
});
