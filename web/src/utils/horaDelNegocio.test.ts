import { fechaYHora, soloHora, soloFecha, zonaSegura, zonaEsUsable } from './horaDelNegocio';

// LA HORA QUE SE MUESTRA ES LA DEL NEGOCIO, NO LA DE LA TABLETA.
//
// Cada pantalla llamaba a `toLocaleString` sin zona, así que decía la hora del navegador. Dos
// Surface con el reloj distinto mostraban horas distintas del mismo pedido, y el ticket que se
// lleva el cliente llevaba la hora de la máquina.
//
// El instante de prueba está elegido a propósito: 2026-09-02 02:25 UTC es 2026-09-01 20:25 en
// México. Es el caso que separa las dos zonas por DÍA, no solo por hora — a cualquier otra, el
// test pasaría con el defecto puesto.
const instante = '2026-09-02T02:25:00Z';

test('formatea en la zona del negocio, no en la del entorno', () => {
  const mx = fechaYHora(instante, 'America/Mexico_City');
  const tokio = fechaYHora(instante, 'Asia/Tokyo');

  // Lo que se comprueba es que cambian de DÍA, no solo de hora: es lo que separa "la zona se
  // aplicó" de "casualmente coinciden".
  expect(mx).toContain('1 sep');   // 20:25 del día 1
  expect(tokio).toContain('2 sep'); // 11:25 del día 2
  expect(mx).not.toBe(tokio);
});

test('la hora sola también', () => {
  // es-MX usa 12 horas con p.m., que es como se lee la hora en el país. Lo que importa es que sea
  // las 8:25 de la noche del 1 y no las 2:25 de la madrugada del 2, que es lo que decía en UTC.
  expect(soloHora(instante, 'America/Mexico_City')).toMatch(/8:25\s*p/);
  expect(soloHora(instante, 'Asia/Tokyo')).toMatch(/11:25\s*a/);
});

test('la fecha sola también', () => {
  expect(soloFecha(instante, 'America/Mexico_City')).toContain('1 sep');
  expect(soloFecha(instante, 'Asia/Tokyo')).toContain('2 sep');
});

// Una zona que el navegador no reconoce no puede tumbar la pantalla ni caer a UTC en silencio: eso
// correría la hora seis horas y se vería plausible, que es el peor modo de fallo posible.
test('una zona que el navegador no reconoce cae al default y no lanza', () => {
  expect(() => fechaYHora(instante, 'Marte/Olympus_Mons')).not.toThrow();
  expect(fechaYHora(instante, 'Marte/Olympus_Mons')).toBe(fechaYHora(instante, 'America/Mexico_City'));
  expect(zonaSegura('Marte/Olympus_Mons')).toBe('America/Mexico_City');
  expect(zonaEsUsable('Marte/Olympus_Mons')).toBe(false);
});

// Sin zona configurada se usa el default del PRODUCTO, nunca la del navegador: si cada tableta cae a
// la suya, el problema vuelve por la puerta de atrás.
test('sin zona se usa el default del producto', () => {
  expect(zonaSegura(undefined)).toBe('America/Mexico_City');
  expect(zonaSegura('')).toBe('America/Mexico_City');
});

// Una fecha inválida se pinta vacía, no como "Invalid Date" en medio de una tabla.
test('una fecha inválida se pinta vacía', () => {
  expect(fechaYHora('no soy una fecha', 'America/Mexico_City')).toBe('');
  expect(fechaYHora(null, 'America/Mexico_City')).toBe('');
  expect(soloHora(undefined, 'America/Mexico_City')).toBe('');
});
