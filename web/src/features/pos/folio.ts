// Los nombres con los que se canta un pedido en cocina, en lugar de su número.
//
// ESTA LISTA ES COPIA de `animales` en server/internal/domain/folio.go, y un test de Go compara
// los dos archivos: si divergen, falla nombrando el animal que sobra o falta. Vive duplicada
// porque la pantalla le pone nombre a la cuenta al ABRIRLA —para que el operador lo vea desde el
// primer producto y no solo al cobrar— y pedirle ese nombre al servidor sería un viaje de red por
// cada cuenta nueva, en la pantalla que más se usa.
//
// Las tres reglas que la gobiernan y por qué, en folio.go. Resumen: ninguno comparte sílaba
// inicial, ninguno es comida, ninguno pasa de nueve letras.
export const ANIMALES = [
  'Águila', 'Alce', 'Ardilla', 'Avestruz', 'Ballena', 'Bisonte', 'Búfalo', 'Búho', 'Burro',
  'Caimán', 'Camello', 'Canguro', 'Castor', 'Cebra', 'Chita', 'Cisne', 'Colibrí', 'Coyote',
  'Cuervo', 'Delfín', 'Dingo', 'Dragón', 'Erizo', 'Foca', 'Gacela', 'Ganso', 'Gaviota',
  'Gorila', 'Halcón', 'Hiena', 'Iguana', 'Jabalí', 'Jaguar', 'Jirafa', 'Koala', 'Lagarto',
  'León', 'Libélula', 'Lince', 'Llama', 'Lobo', 'Loro', 'Lechuza', 'Mamut', 'Mandril',
  'Mapache', 'Mono', 'Morsa', 'Mula', 'Marmota', 'Nutria', 'Ocelote', 'Orca', 'Oso', 'Panda',
  'Pelícano', 'Pingüino', 'Puma', 'Quetzal', 'Reno', 'Sapo', 'Suricata', 'Tapir', 'Tejón',
  'Tiburón', 'Tigre', 'Topo', 'Tortuga', 'Tucán', 'Urraca', 'Vicuña', 'Zorro', 'Alpaca',
  'Antílope', 'Armadillo', 'Cachalote', 'Comadreja', 'Elefante', 'Flamenco', 'Garza',
  'Guacamaya', 'Jilguero', 'Lémur', 'Paloma', 'Petirrojo', 'Capibara', 'Cocodrilo', 'Perico',
  'Ajolote', 'Coatí', 'Grulla', 'Hurón', 'Mirlo', 'Cigüeña', 'Gecko', 'Impala', 'Emú', 'Yegua',
  'Wallaby', 'Orangután',
] as const;

// nombreLibre elige un animal que no estén usando las cuentas abiertas.
//
// Solo evita chocar con lo que la pantalla ve; los pedidos ya cobrados del día no los conoce. Ese
// choque lo resuelve el servidor agregando la vuelta ("Tigre 2"), que conserva el animal —quien
// pidió ya lo oyó— en vez de saltar a otro.
export function nombreLibre(usados: string[], azar: () => number = Math.random): string {
  const tomados = new Set(usados);
  const libres = ANIMALES.filter((a) => !tomados.has(a));
  // Con las 100 cuentas abiertas a la vez —que no pasa— se repite uno y el servidor lo desempata.
  const pool = libres.length > 0 ? libres : ANIMALES;
  return pool[Math.floor(azar() * pool.length)];
}
