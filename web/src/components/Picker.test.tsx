import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from './ui/provider';
import { Picker } from './Picker';

function wrap(ui: React.ReactElement) {
  return render(<Provider>{ui}</Provider>);
}

const sups = [{ value: '1', label: 'Sams Coacalco' }, { value: '2', label: 'Walmart' }];

// El alta inline es la única forma de dar de alta un proveedor sin abandonar la captura del gasto.
// Si se rompe, el operador pierde el borrador entero para irse a la pestaña Proveedores.
test('crear desde el picker da de alta y deja el nuevo elemento seleccionado', async () => {
  const u = userEvent.setup();
  const onChange = vi.fn();
  const onCreate = vi.fn(async (name: string) => ({ value: '9', label: name }));
  wrap(<Picker value="" options={sups} onChange={onChange} onCreate={onCreate} title="Proveedor" />);

  await u.click(screen.getByRole('button', { name: /seleccionar/i }));
  await u.type(screen.getByPlaceholderText('Buscar…'), 'Bimbo');
  await u.click(screen.getByRole('button', { name: /Crear «Bimbo»/ }));

  expect(onCreate).toHaveBeenCalledWith('Bimbo');
  expect(onChange).toHaveBeenCalledWith('9');
});

test('la hoja anuncia el alta inline antes de escribir, y no ofrece crear un duplicado', async () => {
  const u = userEvent.setup();
  const onCreate = vi.fn(async (name: string) => ({ value: '9', label: name }));
  wrap(<Picker value="" options={sups} onChange={vi.fn()} onCreate={onCreate} title="Proveedor" />);

  await u.click(screen.getByRole('button', { name: /seleccionar/i }));
  expect(screen.getByText(/¿No está en la lista\?/)).toBeInTheDocument();

  // Nombre que ya existe: se filtra la fila y NO aparece «Crear» (evita proveedores duplicados).
  await u.type(screen.getByPlaceholderText('Buscar…'), 'walmart');
  expect(screen.queryByRole('button', { name: /Crear/ })).not.toBeInTheDocument();
});

test('sin onCreate no se insinúa un alta que no existe', async () => {
  const u = userEvent.setup();
  wrap(<Picker value="" options={sups} onChange={vi.fn()} title="Proveedor" searchThreshold={1} />);
  await u.click(screen.getByRole('button', { name: /seleccionar/i }));
  await u.type(screen.getByPlaceholderText('Buscar…'), 'Bimbo');
  expect(screen.getByText('Sin resultados.')).toBeInTheDocument();
  expect(screen.queryByText(/¿No está en la lista\?/)).not.toBeInTheDocument();
});

// EL PISO DE 44 PX NO ES NEGOCIABLE POR TAMAÑO.
//
// La receta del tema sube el alto mínimo solo en `size="md"`, así que un Picker `sm` —el que usan
// las barras de filtros— quedaba en 32 px. El tamaño decide la tipografía y el padding; el alto con
// el que un dedo acierta a la primera lo decide el dedo.
test('el disparador mide al menos 44 px aunque el tamaño sea sm', () => {
  wrap(<Picker size="sm" value="" options={[{ value: 'a', label: 'Uno' }]} onChange={() => {}} placeholder="Elegir" />);
  const boton = screen.getByRole('button', { name: /Elegir/ });
  expect(boton).toHaveStyle({ minHeight: '44px' });
});
