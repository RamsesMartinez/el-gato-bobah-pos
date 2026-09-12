import { describe, expect, it } from 'vitest';
import { CELDAS, celdaDelToque } from './celda';

describe('de un toque a una celda', () => {
  it('la esquina superior izquierda es la celda 0', () => {
    expect(celdaDelToque(0, 0, 1024, 600)).toEqual({ celda: 0, orientacion: 'horizontal' });
  });

  it('la esquina inferior derecha es la última, no una que no existe', () => {
    // Un toque exactamente en el borde daría el índice siguiente al último si no se acotara, y ese
    // toque se perdería. Acotar es más honesto que descartar.
    expect(celdaDelToque(1024, 600, 1024, 600)).toEqual({ celda: CELDAS - 1, orientacion: 'horizontal' });
  });

  it('el centro cae en el centro', () => {
    const z = celdaDelToque(512, 300, 1024, 600)!;
    // Columna 6 de 12, fila 3 de 7 → 3*12+6
    expect(z.celda).toBe(42);
  });

  it('en vertical la rejilla se voltea', () => {
    const z = celdaDelToque(0, 0, 600, 1024)!;
    expect(z.orientacion).toBe('vertical');
    // Y sigue habiendo 84 celdas: el mismo número, otra forma.
    const ultima = celdaDelToque(600, 1024, 600, 1024)!;
    expect(ultima.celda).toBe(CELDAS - 1);
  });

  it('EL MISMO PUNTO DEL VIDRIO DA LA MISMA CELDA, esté como esté desplazada la página', () => {
    // Es FR-004, y el test tiene dientes porque compara las DOS lecturas posibles del mismo toque:
    // la del vidrio (`clientY`) y la del documento (`pageY` = clientY + desplazamiento). Con la
    // lista de productos desplazada 400 px, el dedo cae en el mismo lugar físico pero el documento
    // dice otra cosa — y la rejilla mediría contenido en vez de mano.
    const DESPLAZAMIENTO = 400;
    const deVidrio = celdaDelToque(300, 200, 1024, 600);
    const deDocumento = celdaDelToque(300, 200 + DESPLAZAMIENTO, 1024, 600);
    expect(deVidrio).not.toEqual(deDocumento);

    // Y lo que fija el requisito: con la página en otra posición, el mismo punto del vidrio sigue
    // dando la misma celda, porque el desplazamiento no entra en la cuenta ni puede entrar.
    expect(celdaDelToque(300, 200, 1024, 600)).toEqual(deVidrio);
  });

  it('una pantalla de tamaño cero no produce celda', () => {
    expect(celdaDelToque(10, 10, 0, 600)).toBeNull();
  });

  it('dos tabletas de tamaños distintos dan la misma celda para el mismo punto relativo', () => {
    // Es FR-005: la posición se guarda relativa, así que un cuarto de pantalla es un cuarto de
    // pantalla en las dos.
    const chica = celdaDelToque(256, 150, 1024, 600);
    const grande = celdaDelToque(512, 300, 2048, 1200);
    expect(grande).toEqual(chica);
  });
});
