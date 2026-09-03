import { describe, expect, it } from 'vitest';
import { estadoDeOrden } from './reordenarOpciones';

describe('cuándo se pueden reordenar las opciones', () => {
  it('con la lista completa a la vista, sí', () => {
    expect(estadoDeOrden(false, 0)).toEqual({ puedeReordenar: true, motivo: '' });
  });

  // Arrastrar sobre una lista filtrada guardaría ese orden como el real y mandaría al fondo las
  // opciones que no estaban en pantalla.
  it('buscando, no', () => {
    const e = estadoDeOrden(true, 0);
    expect(e.puedeReordenar).toBe(false);
    expect(e.motivo).not.toBe('');
  });

  it('con archivadas ocultas, tampoco', () => {
    const e = estadoDeOrden(false, 3);
    expect(e.puedeReordenar).toBe(false);
    expect(e.motivo).not.toBe('');
  });

  // El motivo tiene que decir QUÉ HACER, no qué pasó: el operador está viendo desaparecer el
  // arrastre y necesita saber cómo recuperarlo, no un diagnóstico.
  it('el motivo dice qué hacer, y distingue las dos causas', () => {
    expect(estadoDeOrden(true, 0).motivo).toContain('buscador');
    expect(estadoDeOrden(false, 2).motivo).toContain('archivadas');
  });

  // Buscando Y con archivadas ocultas gana el buscador: es lo que el operador acaba de hacer, y
  // mandarlo a mostrar archivadas no le devolvería el arrastre.
  it('con las dos causas manda la del buscador', () => {
    expect(estadoDeOrden(true, 5).motivo).toContain('buscador');
  });
});
