import { describe, it, expect } from 'vitest';
import {
  round2, parseMonto, dividirEnPartes, sugerenciasDeMonto, validarCobro,
  cambioDeEfectivo, billetesUtiles, propinaValida, presetsDePropina, type Entrada,
} from './cobro';

// Esta es la aritmética de dinero de las dos hojas de cobro, y hasta ahora no tenía un solo check
// runnable: vivía inline en los componentes, en cinco copias de `round2` y dos parsers distintos del
// mismo campo. Cada caso nombra el defecto concreto que atrapa; el que no nombra ninguno sobra.

describe('parseMonto', () => {
  it('el campo vacío es "pagó justo", no "entregó cero"', () => {
    // El aviso de efectivo insuficiente compara recibido < a cubrir. Sin distinguir ausente de cero,
    // el campo vacío —el caso más común, "Exacto"— deja el botón de cobrar muerto para siempre.
    expect(parseMonto('')).toEqual({ estado: 'ausente' });
    expect(parseMonto('   ')).toEqual({ estado: 'ausente' });
  });

  it('rechaza la coma de millar en vez de leerla como 1', () => {
    // parseFloat('1,000') es 1. En la tableta el operador teclea la coma por costumbre y cobra $999
    // de menos, sin que nada lo delate.
    expect(parseMonto('1,000')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseMonto('1,000.50')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });

  it('rechaza lo que no es un número en vez de disolverlo en cero', () => {
    // Number('abc') es NaN y se disolvía en `cambio = 0` con el botón todavía cobrable.
    expect(parseMonto('abc')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseMonto('12abc')).toEqual({ estado: 'invalido', motivo: 'formato' });
    expect(parseMonto('.')).toEqual({ estado: 'invalido', motivo: 'formato' });
  });

  it('rechaza el negativo en vez de clamparlo por detrás', () => {
    // El Input seguía pintando -50 mientras el cálculo usaba 0: dos cifras de lo mismo en la misma
    // pantalla, que es el defecto que ya dejó una barra diciendo $2,141 y su lista $1,928.
    expect(parseMonto('-50')).toEqual({ estado: 'invalido', motivo: 'negativo' });
  });

  it('acepta lo que sí es dinero, redondeado a dos decimales', () => {
    expect(parseMonto('0')).toEqual({ estado: 'valido', valor: 0 });
    expect(parseMonto('460')).toEqual({ estado: 'valido', valor: 460 });
    expect(parseMonto('33.335')).toEqual({ estado: 'valido', valor: 33.34 });
    expect(parseMonto(' 100.5 ')).toEqual({ estado: 'valido', valor: 100.5 });
  });
});

describe('dividirEnPartes', () => {
  it('el residuo va a la última parte y la suma es exacta', () => {
    // 100/3 redondeado son tres partes de 33.33 = 99.99. El servidor tolera ese centavo y CIERRA el
    // pedido, así que quedaba un centavo que nadie podía cobrar y que el tablero seguía sumando.
    const partes = dividirEnPartes(100, 3);
    expect(partes).toEqual([33.33, 33.33, 33.34]);
    expect(partes!.reduce((a, b) => a + b, 0)).toBeCloseTo(100, 10);
  });

  it('reparte parejo cuando sí divide', () => {
    expect(dividirEnPartes(500, 2)).toEqual([250, 250]);
    expect(dividirEnPartes(0.03, 3)).toEqual([0.01, 0.01, 0.01]);
  });

  it('rechaza una división que dejaría una parte en cero', () => {
    expect(dividirEnPartes(0.02, 3)).toBeNull();
    expect(dividirEnPartes(100, 0)).toBeNull();
    expect(dividirEnPartes(0, 2)).toBeNull();
  });
});

describe('sugerenciasDeMonto', () => {
  it('ofrece todo y las divisiones parejas, para no abrir el teclado', () => {
    // El teclado del sistema come 250 de los 600 px de alto y tapa la cifra que decide si el botón
    // se enciende. Repartir entre dos, tres o cuatro es el caso común y no puede costar teclear.
    expect(sugerenciasDeMonto(500)).toEqual([
      { etiqueta: 'Todo', monto: 500 },
      { etiqueta: 'Entre 2', monto: 250 },
      { etiqueta: 'Entre 3', monto: 166.66 },
      { etiqueta: 'Entre 4', monto: 125 },
    ]);
  });

  it('con un faltante mínimo no ofrece divisiones que cobrarían cero', () => {
    expect(sugerenciasDeMonto(0.02)).toEqual([
      { etiqueta: 'Todo', monto: 0.02 },
      { etiqueta: 'Entre 2', monto: 0.01 },
    ]);
  });
});

const entrada = (over: Partial<Entrada> = {}): Entrada => ({
  monto: '100', metodoId: 1, propina: '', recibido: '', esEfectivo: false,
  falta: 100, totalDelPedido: 100, ...over,
});

describe('validarCobro', () => {
  it('cobrar todo lo que falta, sin efectivo: listo', () => {
    const v = validarCobro(entrada());
    expect(v.ok).toBe(true);
    expect(v.monto).toBe(100);
  });

  it('cobrar una parte también es válido: eso es dividir la cuenta', () => {
    expect(validarCobro(entrada({ monto: '40', falta: 100 })).ok).toBe(true);
  });

  it('avisa del exceso ANTES de mandar', () => {
    // Si no, el cobro sale, el servidor lo rechaza con ErrCobroExcede, y el operador se entera con
    // el dinero del cliente en la mano.
    const v = validarCobro(entrada({ monto: '120', falta: 100 }));
    expect(v.ok).toBe(false);
    expect(v.motivo).toBe('excede');
  });

  it('cobrar exactamente lo que falta se puede; un centavo más, no', () => {
    // El tope es EXACTO, igual que domain.ValidarCobro: la tolerancia del centavo vive en el
    // predicado que CIERRA el pedido, no en el que acota cada cobro. Ser más laxo aquí manda un
    // cobro que el servidor rechaza, y el operador se entera con el dinero en la mano.
    expect(validarCobro(entrada({ monto: '33.33', falta: 33.33 })).ok).toBe(true);
    expect(validarCobro(entrada({ monto: '33.34', falta: 33.33 })).motivo).toBe('excede');
  });

  it('sin método no se cobra, en vez de mandar el id 0', () => {
    // `metodoId ?? 0` mandaba methodId: 0 y el servidor respondía 404 sin decirle nada útil a quien
    // está cobrando con el cliente enfrente.
    const v = validarCobro(entrada({ metodoId: null }));
    expect(v.ok).toBe(false);
    expect(v.motivo).toBe('sin-metodo');
  });

  it('un monto mal escrito bloquea, no cae a cero', () => {
    expect(validarCobro(entrada({ monto: '1,000', falta: 1000 })).motivo).toBe('monto-invalido');
    expect(validarCobro(entrada({ monto: '' })).motivo).toBe('sin-monto');
  });

  it('la propina no puede superar la cuenta entera', () => {
    // Espejo de domain.ValidarPropina: el rebote se ve antes de tener el dinero en la mano.
    const v = validarCobro(entrada({ propina: '9999', totalDelPedido: 100 }));
    expect(v.ok).toBe(false);
    expect(v.motivo).toBe('propina-excede');
  });

  it('en efectivo el campo vacío es exacto y se cobra', () => {
    // Es el caso más común del mostrador. Tratar el vacío como $0 recibidos lo dejaba muerto.
    const v = validarCobro(entrada({ esEfectivo: true, recibido: '' }));
    expect(v.ok).toBe(true);
    expect(v.cambio).toBe(0);
  });

  it('en efectivo avisa si lo recibido no alcanza, y no deja cobrar', () => {
    const v = validarCobro(entrada({ monto: '100', esEfectivo: true, recibido: '50' }));
    expect(v.ok).toBe(false);
    expect(v.motivo).toBe('falta-efectivo');
    expect(v.faltaEfectivo).toBe(50);
  });

  it('el efectivo tiene que cubrir el monto MÁS la propina', () => {
    // Cobrar $100 con $20 de propina y $110 recibidos deja $10 que el cliente no dio, y el cajón los
    // espera igual porque la propina entra al esperado del corte.
    const v = validarCobro(entrada({ monto: '100', propina: '20', esEfectivo: true, recibido: '110' }));
    expect(v.ok).toBe(false);
    expect(v.faltaEfectivo).toBe(10);
  });

  it('calcula el cambio contra monto más propina', () => {
    const v = validarCobro(entrada({ monto: '100', propina: '20', esEfectivo: true, recibido: '200' }));
    expect(v.ok).toBe(true);
    expect(v.cambio).toBe(80);
  });
});

describe('cambioDeEfectivo', () => {
  it('el campo vacío es exacto: ni cambio ni faltante', () => {
    expect(cambioDeEfectivo('', 460)).toEqual({ exacto: true, cambio: 0, falta: 0 });
  });

  it('calcula el cambio sin arrastrar los flotantes', () => {
    // La resta cruda daba $0.010000000000005 en cuanto había un decimal.
    expect(cambioDeEfectivo('500', 460.1)).toEqual({ exacto: false, cambio: 39.9, falta: 0 });
  });

  it('un monto ilegible no se toma como cero', () => {
    expect(cambioDeEfectivo('abc', 460)).toEqual({ exacto: false, cambio: 0, falta: 0, invalido: true });
  });
});

describe('billetesUtiles', () => {
  it('solo los billetes con los que alcanza', () => {
    // Pintarlos todos deja tocar un $50 sobre $175, que mete la pantalla en "falta efectivo" y
    // deshabilita el botón: un tap que solo sirve para trabar el cobro.
    expect(billetesUtiles(175)).toEqual([200, 500, 1000]);
    expect(billetesUtiles(50)).toEqual([50, 100, 200, 500, 1000]);
  });

  it('con un monto grande no queda ninguno y la fila no se pinta', () => {
    expect(billetesUtiles(5000)).toEqual([]);
  });
});

describe('presetsDePropina', () => {
  it('el porcentaje es de lo que se cobra ahora, no del total del pedido', () => {
    // Sobre el total, un pedido de $500 con $300 abonados ofrecía "15%" = $75 en una pantalla que
    // dice Cobrar $200: 37.5% de la cifra que el operador tiene enfrente, y sin error que lo delate
    // porque pasa el tope.
    expect(presetsDePropina(200)).toEqual([
      { etiqueta: '10%', monto: 20 },
      { etiqueta: '15%', monto: 30 },
      { etiqueta: '20%', monto: 40 },
    ]);
  });
});

describe('propinaValida', () => {
  it('es espejo de ValidarPropina del servidor', () => {
    expect(propinaValida(0, 250)).toBe(true);
    expect(propinaValida(250, 250)).toBe(true);
    expect(propinaValida(251, 250)).toBe(false);
  });
});

describe('round2', () => {
  it('redondea a dos decimales en todas las fronteras', () => {
    expect(round2(0.1 + 0.2)).toBe(0.3);
    expect(round2(33.335)).toBe(33.34);
    expect(round2(1.005)).toBe(1.01);
  });
});
