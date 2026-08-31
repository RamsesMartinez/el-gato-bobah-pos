import { describe, it, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { buildReceiptHtml, overflowingLines, printFrame, printHtmlOffscreen, sampleTicketOrder, TICKET_COLUMNS, type TicketBusinessInfo } from './printReceipt';
import { money } from './format';
import type { ReceiptOrder } from '../types/pos';

const baseBusiness: TicketBusinessInfo = {
  businessName: 'El Gato Bobah',
  address: 'Av. Siempre Viva 742',
  phone: '55 1234 5678',
  headerNote: 'Wi-Fi: gatobobah',
  footerNote: '¡Gracias por su compra!',
  logoDataUri: 'data:image/webp;base64,QUFB',
};

const baseOrder: ReceiptOrder = {
  folioName: 'Tigre',
  id: 1,
  number: 42,
  status: 'abierta',
  serviceType: 'mostrador',
  customerName: null,
  subtotal: '5000',
  deliveryFee: '0',
  total: '5000',
  currency: 'MXN',
  paid: false,
  openedAt: '2026-07-19T12:00:00Z',
  lines: [],
};

describe('buildReceiptHtml', () => {
  it('escapes attacker-controlled strings so they cannot inject markup/script', () => {
    const order: ReceiptOrder = {
      ...baseOrder,
      customerName: '<img src=x onerror=alert(1)>',
      lines: [
        {
          productName: '<script>alert(2)</script>',
          quantity: '1',
          unitPrice: '5000',
          lineTotal: '5000',
          modifiers: [{ name: '<b>extra</b>', quantity: 1, priceDelta: '0' }],
        },
      ],
    };
    const html = buildReceiptHtml(order, baseBusiness);

    // No live markup from user data
    expect(html).not.toContain('<img src=x');
    expect(html).not.toContain('<script>alert(2)');
    expect(html).not.toContain('<b>extra</b>');
    // Escaped equivalents are present
    expect(html).toContain('&lt;img src=x');
    expect(html).toContain('&lt;script&gt;alert(2)');
  });

  it('renders normal order data verbatim (no double-escaping of safe text)', () => {
    const order: ReceiptOrder = {
      ...baseOrder,
      customerName: 'María',
      lines: [{ productName: 'Boba fresa', quantity: '2', unitPrice: '2500', lineTotal: '5000', modifiers: [] }],
    };
    const html = buildReceiptHtml(order, baseBusiness);
    expect(html).toContain('María');
    expect(html).toContain('Boba fresa');
    expect(html).toContain('Pedido #42');
  });
});

describe('buildReceiptHtml — encabezado del negocio', () => {
  it('imprime la identidad, el contacto y el logo', () => {
    const html = buildReceiptHtml(baseOrder, baseBusiness);
    expect(html).toContain('El Gato Bobah');
    expect(html).toContain('Av. Siempre Viva 742');
    expect(html).toContain('55 1234 5678');
    expect(html).toContain('¡Gracias por su compra!');
    // El logo va incrustado, no referenciado: un <img src> remoto lo bloquea la CSP de producción
    // (img-src 'self' data:) y además puede no haber cargado cuando se dispara print().
    expect(html).toContain('src="data:image/webp;base64,QUFB"');
  });

  it('omite los renglones opcionales vacíos en vez de dejar huecos', () => {
    const html = buildReceiptHtml(baseOrder, { ...baseBusiness, address: '', phone: '', footerNote: '' });
    expect(html).toContain('El Gato Bobah');
    expect(html).not.toContain('Av. Siempre Viva');
    expect(html).not.toContain('55 1234 5678');
    // Sin leyenda propia, el ticket cierra con el saludo por default.
    expect(html).toContain('¡Gracias!');
  });

  it('escapa los datos del negocio igual que los del pedido', () => {
    // El documento del ticket vive en un iframe srcdoc, o sea MISMO ORIGEN: un onerror en el
    // nombre del negocio correría con acceso al token en localStorage.
    const html = buildReceiptHtml(baseOrder, {
      ...baseBusiness,
      businessName: '<img src=x onerror=alert(1)>',
      footerNote: '<script>alert(2)</script>',
    });
    expect(html).not.toContain('<img src=x');
    expect(html).not.toContain('<script>alert(2)');
    expect(html).toContain('&lt;img src=x');
  });
});

describe('buildReceiptHtml — reimpresión', () => {
  it('marca el papel cuando es reimpresión', () => {
    expect(buildReceiptHtml(baseOrder, baseBusiness, { reprint: true })).toContain('REIMPRESIÓN');
  });

  it('no marca el ticket original', () => {
    expect(buildReceiptHtml(baseOrder, baseBusiness)).not.toContain('REIMPRESIÓN');
  });
});

describe('buildReceiptHtml — dinero', () => {
  it('imprime los importes del pedido sin recalcularlos', () => {
    // El total NO cuadra con las líneas a propósito: el servidor es la única fuente de verdad de
    // los precios y el ticket no debe "corregirlo" (FR-014). Si el builder sumara, $999 no
    // aparecería en ninguna parte del documento.
    const order: ReceiptOrder = {
      ...baseOrder,
      subtotal: '100',
      total: '999',
      lines: [{ productName: 'Ramen', quantity: '1', unitPrice: '100', lineTotal: '100', modifiers: [] }],
    };
    expect(buildReceiptHtml(order, baseBusiness)).toContain(money('999'));
  });
});

describe('printFrame', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    document.querySelectorAll('iframe').forEach((f) => f.remove());
  });

  function mountFrame() {
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    const print = vi.fn();
    // jsdom no implementa print(); se reemplaza para poder contar las llamadas.
    Object.defineProperty(frame.contentWindow!, 'print', { value: print, writable: true });
    Object.defineProperty(frame.contentWindow!, 'focus', { value: vi.fn(), writable: true });
    return { frame, print };
  }

  it('imprime el documento del iframe que ya está montado', () => {
    const { frame, print } = mountFrame();
    expect(printFrame(frame)).toBe(true);
    expect(print).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(2000);
  });

  it('un segundo toque inmediato no manda un segundo trabajo', () => {
    // Dos tickets por un doble toque son papel desperdiciado y un comprobante duplicado en la
    // mano del cliente; el candado dura lo que tarda el diálogo del navegador en aparecer.
    const { frame, print } = mountFrame();
    expect(printFrame(frame)).toBe(true);
    expect(printFrame(frame)).toBe(false);
    expect(print).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(2000);
  });

  it('vuelve a permitir imprimir cuando pasó la ventana del candado', () => {
    const { frame, print } = mountFrame();
    printFrame(frame);
    vi.advanceTimersByTime(2000);
    expect(printFrame(frame)).toBe(true);
    expect(print).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(2000);
  });

  it('no revienta cuando todavía no hay iframe', () => {
    expect(printFrame(null)).toBe(false);
  });
});

describe('buildReceiptHtml — legibilidad en térmica', () => {
  function css(html: string) {
    return html.slice(html.indexOf('<style>'), html.indexOf('</style>'));
  }

  it('no declara ningún color que no sea negro puro', () => {
    // La impresora térmica es de 1 bit: cualquier gris se convierte en un patrón de puntos
    // salteados y el texto sale desvaído. La jerarquía visual tiene que venir del tamaño y del
    // peso, nunca del color.
    const colores = [...css(buildReceiptHtml(baseOrder, baseBusiness)).matchAll(/[^-]color:\s*([^;]+);/g)]
      .map((m) => m[1].trim());
    expect(colores.length).toBeGreaterThan(0);
    expect(colores.filter((c) => c !== '#000')).toEqual([]);
  });

  it('imprime en negritas: en térmica el trazo delgado no se distingue', () => {
    expect(css(buildReceiptHtml(baseOrder, baseBusiness))).toMatch(/body\s*\{[^}]*font-weight:\s*bold/);
  });
});

describe('buildReceiptHtml — texto superior', () => {
  it('imprime el texto superior arriba del detalle del pedido', () => {
    const html = buildReceiptHtml(baseOrder, { ...baseBusiness, headerNote: 'Wi-Fi: gatobobah' });
    expect(html).toContain('Wi-Fi: gatobobah');
    // Arriba del detalle, no en cualquier lado: va antes de la línea que separa el encabezado.
    expect(html.indexOf('Wi-Fi: gatobobah')).toBeLessThan(html.indexOf('<table>'));
  });

  it('omite el renglón cuando no hay texto superior', () => {
    expect(buildReceiptHtml(baseOrder, { ...baseBusiness, headerNote: '' })).not.toContain('class="note"');
  });

  it('escapa el texto superior', () => {
    const html = buildReceiptHtml(baseOrder, { ...baseBusiness, headerNote: '<img src=x onerror=alert(1)>' });
    expect(html).not.toContain('<img src=x');
    expect(html).toContain('&lt;img src=x');
  });
});

describe('printHtmlOffscreen', () => {
  // Relojes falsos y avance al final de cada caso: printFrame guarda su candado anti-doble-toque
  // en el módulo, así que sin esto un test deja bloqueado al siguiente.
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.advanceTimersByTime(60_000);
    vi.useRealTimers();
    document.querySelectorAll('iframe').forEach((f) => f.remove());
  });

  it('imprime el documento fuera de pantalla y no deja el iframe montado', async () => {
    const doc = '<!doctype html><html><body>ticket</body></html>';
    const promesa = printHtmlOffscreen(doc);

    // El iframe se monta de forma síncrona: si no, no habría a qué engancharle el load.
    const frame = document.querySelector('iframe');
    expect(frame).not.toBeNull();
    expect(frame!.getAttribute('srcdoc')).toBe(doc);

    const print = vi.fn();
    Object.defineProperty(frame!.contentWindow!, 'print', { value: print, writable: true });
    Object.defineProperty(frame!.contentWindow!, 'focus', { value: vi.fn(), writable: true });

    // Imprimir ANTES del load saca papel en blanco: el documento todavía no existe.
    expect(print).not.toHaveBeenCalled();
    frame!.contentDocument!.body.innerHTML = '<p>ticket</p>';
    frame!.dispatchEvent(new Event('load'));

    await expect(promesa).resolves.toBe(true);
    expect(print).toHaveBeenCalledTimes(1);
  });
});

describe('buildReceiptHtml — textos de varios renglones', () => {
  const bloque = '=============\nTICKET SIN VALOR FISCAL\n=============\nfacturacion@elgatobobah.com';

  it('respeta los saltos de línea del texto inferior', () => {
    const html = buildReceiptHtml(baseOrder, { ...baseBusiness, footerNote: bloque });
    // Sin pre-line, el HTML colapsa los saltos y el aviso sale como un párrafo corrido.
    expect(html).toMatch(/\.note[^}]*white-space:\s*pre-line/);
    expect(html).toContain('TICKET SIN VALOR FISCAL');
    expect(html).toContain('facturacion@elgatobobah.com');
    // El texto conserva sus saltos: el navegador es quien los pinta como renglones.
    expect(html).toContain(bloque);
  });

  it('sigue escapando aunque venga en varios renglones', () => {
    const html = buildReceiptHtml(baseOrder, { ...baseBusiness, footerNote: 'linea 1\n<script>alert(1)</script>' });
    expect(html).not.toContain('<script>alert(1)');
    expect(html).toContain('&lt;script&gt;');
  });
});

describe('buildReceiptHtml — ticket de prueba', () => {
  it('marca el papel como prueba', () => {
    // Un ticket de prueba que llega a manos de un cliente parece una venta. La marca es lo que
    // lo distingue en el papel, igual que la de reimpresión.
    expect(buildReceiptHtml(baseOrder, baseBusiness, { sample: true })).toContain('TICKET DE PRUEBA');
  });

  it('no marca un ticket normal', () => {
    expect(buildReceiptHtml(baseOrder, baseBusiness)).not.toContain('TICKET DE PRUEBA');
  });

  it('el pedido de muestra ejercita el encabezado, las líneas y los totales', () => {
    const html = buildReceiptHtml(sampleTicketOrder(), baseBusiness, { sample: true });
    expect(html).toContain('El Gato Bobah');
    expect(html).toMatch(/<tr><td>\d+x /); // al menos una línea con cantidad
    expect(html).toContain('TOTAL');
  });
});

describe('overflowingLines', () => {
  it('señala los renglones que no caben a lo ancho del papel', () => {
    const texto = ['corto', 'x'.repeat(TICKET_COLUMNS), 'y'.repeat(TICKET_COLUMNS + 1), 'otro corto'].join('\n');
    // Renglón 3 (1-based): es el único que se pasa. El que mide justo el ancho SÍ cabe.
    expect(overflowingLines(texto)).toEqual([3]);
  });

  it('no señala nada cuando todo cabe', () => {
    expect(overflowingLines('uno\ndos\ntres')).toEqual([]);
  });

  it('texto vacío no señala nada', () => {
    expect(overflowingLines('')).toEqual([]);
  });

  it('cuenta caracteres, no bytes: los acentos no acortan el renglón', () => {
    // "ñ" ocupa dos bytes; medir en bytes marcaría como largo un renglón que sí cabe.
    expect(overflowingLines('ñ'.repeat(TICKET_COLUMNS))).toEqual([]);
  });
});

describe('printHtmlOffscreen — trampas del navegador real', () => {
  beforeEach(() => vi.useFakeTimers());

  function frameActual() {
    return document.querySelector('iframe');
  }
  function stubPrint(frame: HTMLIFrameElement) {
    const print = vi.fn();
    Object.defineProperty(frame.contentWindow!, 'print', { value: print, writable: true });
    Object.defineProperty(frame.contentWindow!, 'focus', { value: vi.fn(), writable: true });
    return print;
  }
  function llenarDocumento(frame: HTMLIFrameElement) {
    frame.contentDocument!.body.innerHTML = '<h1>El Gato Bobah</h1>';
  }

  afterEach(() => {
    vi.advanceTimersByTime(60_000);
    vi.useRealTimers();
    document.querySelectorAll('iframe').forEach((f) => f.remove());
  });

  it('ignora el load del about:blank inicial y no imprime un documento vacío', () => {
    // Un iframe recién insertado dispara `load` por su about:blank ANTES de cargar el srcdoc.
    // Imprimir ahí saca papel en blanco y, peor, desmonta el iframe antes de que llegue el ticket.
    void printHtmlOffscreen('<!doctype html><html><body>ticket</body></html>');
    const frame = frameActual()!;
    const print = stubPrint(frame);

    frame.dispatchEvent(new Event('load')); // documento todavía vacío
    expect(print).not.toHaveBeenCalled();
    expect(frameActual()).not.toBeNull(); // sigue montado, esperando el documento de verdad

    llenarDocumento(frame);
    frame.dispatchEvent(new Event('load'));
    expect(print).toHaveBeenCalledTimes(1);
  });

  it('no desmonta el iframe en el mismo tick en que imprime', async () => {
    // Quitar el iframe justo después de print() cancela el trabajo en algunos navegadores: el
    // diálogo alcanza a abrir pero se queda sin documento que imprimir.
    const p = printHtmlOffscreen('<!doctype html><html><body>ticket</body></html>');
    const frame = frameActual()!;
    stubPrint(frame);
    llenarDocumento(frame);
    frame.dispatchEvent(new Event('load'));

    expect(frameActual()).not.toBeNull();
    vi.advanceTimersByTime(60_000);
    await p;
    expect(frameActual()).toBeNull();
  });
});

describe('desglose de precio en el ticket', () => {
  const conExtras: ReceiptOrder = {
    ...baseOrder,
    lines: [{
      productName: 'Café Americano',
      quantity: '2',
      unitPrice: '50',
      lineTotal: '120',
      modifiers: [
        { name: 'Leche deslactosada', quantity: 1, priceDelta: '10' },
        { name: 'Canela', quantity: 1, priceDelta: '0' },
      ],
    }],
  };

  // El cliente tiene que poder explicarse el número. Antes el renglón mostraba 120.00 con los
  // extras listados sin precio, y no había forma de saber de dónde salían los 20 de más.
  test('el producto muestra su precio unitario y su base, no el total con extras', () => {
    const html = buildReceiptHtml(conExtras, baseBusiness, {});
    // Se comparan contra money() y no contra un literal: el formato del repo omite los decimales
    // cuando son cero ($50, no $50.00), y clavar el literal ataría el test al formato en vez de al
    // comportamiento.
    expect(html).toContain(`@${money('50')}`);
    expect(html).toContain(money('100')); // 2 × 50 base, sin los extras
  });

  test('un extra que cuesta lleva su unitario y su importe', () => {
    const html = buildReceiptHtml(conExtras, baseBusiness, {});
    expect(html).toContain('Leche deslactosada');
    expect(html).toContain(`@${money('10')}`);
    expect(html).toContain(money('20')); // 2 cafés × 10 de delta
  });

  test('un extra sin costo se lista sin cifra', () => {
    const html = buildReceiptHtml(conExtras, baseBusiness, {});
    // El renglón de Canela llega hasta el cierre de su fila: no debe traer ninguna cifra.
    const desde = html.indexOf('Canela');
    const canela = html.slice(desde, html.indexOf('</tr>', desde));
    expect(canela).not.toMatch(/\$\s?[\d,]+/);
  });

  // El negocio puede apagar los gratuitos para no alargar el papel. Los que cuestan nunca se
  // ocultan: son la explicación del total.
  test('con el interruptor apagado, los gratuitos desaparecen y los que cuestan no', () => {
    const html = buildReceiptHtml(conExtras, baseBusiness, { printFreeModifiers: false });
    expect(html).not.toContain('Canela');
    expect(html).toContain('Leche deslactosada');
  });

  test('el total del ticket no cambia por desglosar', () => {
    const html = buildReceiptHtml(conExtras, baseBusiness, {});
    expect(html).toContain(money(conExtras.total));
  });
});
