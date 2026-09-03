import { describe, expect, it } from 'vitest';
import { completarConLaUltima, type Seleccion } from './ultimaCombinacion';
import type { MenuGroup } from '../../types/pos';

const grupo = (id: number, min: number, max: number, opciones: number[]): MenuGroup =>
  ({ id, title: `G${id}`, min, max, options: opciones.map((o) => ({ id: o, name: `O${o}`, priceDelta: '0', maxPerLine: 2, favorite: false })) });

describe('completar con la última combinación', () => {
  const grupos = [grupo(7, 2, 2, [10, 11, 12]), grupo(8, 0, 1, [20])];

  // El caso que cuesta los taps: sin historial el pre-marcado no adivina, así que el grupo queda
  // sin cumplir y el operador elige dos salsas a mano cada vez.
  it('llena un grupo obligatorio que quedó sin cumplir', () => {
    const base: Seleccion = {};
    const guardada: Seleccion = { 7: { 10: 2 } };
    expect(completarConLaUltima(base, guardada, grupos)).toEqual({ 7: { 10: 2 } });
  });

  // Lo que el pre-marcado por historial ya resolvió NO se toca: esa señal es mejor que la memoria
  // de esta tableta, porque sale de lo que de verdad se vendió.
  it('no pisa lo que el pre-marcado ya eligió', () => {
    const base: Seleccion = { 7: { 11: 1, 12: 1 } };
    const guardada: Seleccion = { 7: { 10: 2 } };
    expect(completarConLaUltima(base, guardada, grupos)).toEqual({ 7: { 11: 1, 12: 1 } });
  });

  // Un grupo opcional no se rellena solo: agregaría un extra que nadie pidió y que se cobra.
  it('no toca los grupos opcionales', () => {
    const guardada: Seleccion = { 8: { 20: 1 } };
    expect(completarConLaUltima({}, guardada, grupos)).toEqual({});
  });

  // Una opción archivada desde la última vez ya no existe en el menú: rellenar con ella mandaría a
  // cocina algo que el negocio quitó.
  it('descarta opciones que ya no están en el menú', () => {
    const guardada: Seleccion = { 7: { 99: 2 } };
    expect(completarConLaUltima({}, guardada, grupos)).toEqual({});
  });

  // Si la memoria trae más de lo que el grupo admite, se recorta: el grupo cambió de máximo.
  it('respeta el máximo del grupo', () => {
    const guardada: Seleccion = { 7: { 10: 5 } };
    expect(completarConLaUltima({}, guardada, grupos)).toEqual({ 7: { 10: 2 } });
  });

  it('sin nada guardado deja la selección como estaba', () => {
    const base: Seleccion = { 7: { 11: 1 } };
    expect(completarConLaUltima(base, null, grupos)).toEqual(base);
  });
});
