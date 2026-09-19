import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// FR-008: no se automatiza el portal de comercios de ninguna plataforma.
//
// Los términos mexicanos de Uber prohíben explícitamente «lanzar cualquier programa o script con el
// objeto de extraer, indexar, analizar o de otro modo realizar prospección de datos de cualquier
// parte de los Servicios», y la terminación es inmediata y sin causa pactada. El beneficio serían
// minutos de trabajo a la semana; el costo del peor caso es la cuenta de la que vive el negocio.
//
// Playwright sí está permitido —es el motor de las pruebas de punta a punta contra NUESTRO
// ambiente— y por eso se revisa solo `dependencies`, no `devDependencies`: lo que no puede pasar es
// que un navegador automatizado llegue al paquete que se despliega.
describe('automatización de navegador', () => {
  it('ninguna dependencia de producción trae un navegador', () => {
    const pkg = JSON.parse(readFileSync(resolve(__dirname, '../../package.json'), 'utf8'));
    const prod = Object.keys(pkg.dependencies ?? {});
    const navegadores = ['playwright', 'puppeteer', 'selenium', 'cypress', 'webdriverio'];
    const encontradas = prod.filter((d) => navegadores.some((n) => d.includes(n)));
    expect(encontradas, 'automatizar el portal de una plataforma termina el contrato').toEqual([]);
  });
});
