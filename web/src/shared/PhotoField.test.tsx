import { vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import { PhotoField } from './PhotoField';
import { PhotoError } from '../media/preparePhoto';

const media = vi.hoisted(() => ({
  canPreparePhotos: vi.fn(() => true),
  photoSize: vi.fn(async () => ({ width: 4000, height: 3000 })),
  preparePhoto: vi.fn(async () => ({ large: new Blob(['LL'], { type: 'image/webp' }), small: new Blob(['S'], { type: 'image/webp' }) })),
  toBase64: vi.fn(async (b: Blob) => (b.size === 2 ? 'QQ==' : 'Qg==')),
}));
vi.mock('../media/preparePhoto', async (orig) => ({ ...(await orig<object>()), ...media }));

const api = vi.hoisted(() => ({
  setImage: vi.fn(async () => ({ imageUrl: 'https://media.test/1/products/7/aaaa-1200.webp' })),
  removeImage: vi.fn(async () => ({ imageUrl: null })),
}));
vi.mock('../api/admin', async (orig) => ({ ...(await orig<object>()), adminApi: { ...((await orig<{ adminApi: object }>()).adminApi), ...api } }));

beforeEach(() => {
  vi.clearAllMocks();
  media.canPreparePhotos.mockReturnValue(true);
  URL.createObjectURL = vi.fn(() => 'blob:foto');
  URL.revokeObjectURL = vi.fn();
});

function wrap(props: Partial<React.ComponentProps<typeof PhotoField>> = {}) {
  const onChanged = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Provider>
        <PhotoField kind="products" id={7} name="Taro" imageUrl={null} family="tea" onChanged={onChanged} {...props} />
      </Provider>
    </QueryClientProvider>,
  );
  return { onChanged };
}

const file = new File(['x'], 'taro.jpg', { type: 'image/jpeg' });

test('un botón grande abre el selector; el archivo no es un input visible', async () => {
  wrap();
  const input = document.querySelector('input[type=file]') as HTMLInputElement;
  expect(input.accept).toBe('image/jpeg,image/png,image/webp');
  expect(input.style.display).not.toBe('none'); // Safari táctil no abre un input con display:none
  const click = vi.spyOn(input, 'click');
  await userEvent.click(screen.getByRole('button', { name: /Agregar foto/ }));
  expect(click).toHaveBeenCalled();
});

test('elegir una foto muestra cómo se verá y «Guardar foto» la sube con sus dos medidas', async () => {
  const u = userEvent.setup({ pointerEventsCheck: 0 }); // el input es invisible a propósito
  const { onChanged } = wrap();
  await u.upload(document.querySelector('input[type=file]') as HTMLInputElement, file);
  expect(await screen.findByText('Así se verá')).toBeTruthy();
  await u.click(screen.getByRole('button', { name: 'Guardar foto' }));
  await waitFor(() => expect(api.setImage).toHaveBeenCalledWith('products', 7, { image1200: 'QQ==', image400: 'Qg==' }));
  expect(onChanged).toHaveBeenCalledWith('https://media.test/1/products/7/aaaa-1200.webp');
  expect(await screen.findByText('Foto guardada')).toBeTruthy();
  // Elegir el mismo archivo otra vez tiene que volver a disparar el cambio.
  expect((document.querySelector('input[type=file]') as HTMLInputElement).value).toBe('');
});

test('«Ajustar encuadre» ofrece acercar y alejar; «Cancelar» no sube nada', async () => {
  const u = userEvent.setup({ pointerEventsCheck: 0 }); // el input es invisible a propósito
  wrap();
  await u.upload(document.querySelector('input[type=file]') as HTMLInputElement, file);
  await u.click(await screen.findByRole('button', { name: /Ajustar encuadre/ }));
  expect(screen.getByRole('button', { name: 'Acercar' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Alejar' })).toBeTruthy();
  await u.click(screen.getByRole('button', { name: 'Cancelar' }));
  expect(api.setImage).not.toHaveBeenCalled();
});

test('una foto que no se puede abrir dice qué hacer y no abre el recorte', async () => {
  media.photoSize.mockRejectedValueOnce(new PhotoError('cannot_open'));
  const u = userEvent.setup({ pointerEventsCheck: 0 }); // el input es invisible a propósito
  wrap();
  await u.upload(document.querySelector('input[type=file]') as HTMLInputElement, file);
  expect(await screen.findByText(/Elige una en JPG o PNG/)).toBeTruthy();
  expect(screen.queryByText('Así se verá')).toBeNull();
});

test('un navegador que no puede preparar fotos lo dice antes de que alguien recorte', () => {
  media.canPreparePhotos.mockReturnValue(false);
  wrap();
  expect((screen.getByRole('button', { name: /Agregar foto/ }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText(/Ábrelo en Chrome o Edge/)).toBeTruthy();
});

test('quitar la foto pide confirmación y dice el efecto', async () => {
  const u = userEvent.setup({ pointerEventsCheck: 0 }); // el input es invisible a propósito
  const { onChanged } = wrap({ kind: 'categories', imageUrl: 'https://media.test/1/categories/7/bbbb-1200.webp' });
  await u.click(screen.getByRole('button', { name: 'Quitar foto' }));
  expect(await screen.findByText(/Los productos sin foto propia dejarán de mostrar ésta/)).toBeTruthy();
  await u.click(screen.getByRole('button', { name: 'Quitar' }));
  await waitFor(() => expect(api.removeImage).toHaveBeenCalledWith('categories', 7));
  expect(onChanged).toHaveBeenCalledWith(null);
});

test('si guardar falla, el aviso se ve dentro del recorte y la foto sigue ahí para reintentar', async () => {
  api.setImage.mockRejectedValueOnce(new Error('sin red'));
  const u = userEvent.setup({ pointerEventsCheck: 0 });
  const { onChanged } = wrap();
  await u.upload(document.querySelector('input[type=file]') as HTMLInputElement, file);
  await u.click(await screen.findByRole('button', { name: 'Guardar foto' }));
  const dialog = await screen.findByRole('dialog');
  expect(await within(dialog).findByRole('alert')).toHaveTextContent(/No se pudo guardar la foto/);
  expect(onChanged).not.toHaveBeenCalled();
});

test('mientras se guarda no se puede cancelar: lo que se ve es lo que pasa', async () => {
  let resolve!: (v: { imageUrl: string }) => void;
  api.setImage.mockImplementationOnce(() => new Promise((r) => { resolve = r; }));
  const u = userEvent.setup({ pointerEventsCheck: 0 });
  wrap();
  await u.upload(document.querySelector('input[type=file]') as HTMLInputElement, file);
  await u.click(await screen.findByRole('button', { name: 'Guardar foto' }));
  await waitFor(() => expect((screen.getByRole('button', { name: 'Cancelar' }) as HTMLButtonElement).disabled).toBe(true));
  resolve({ imageUrl: 'https://media.test/1/products/7/aaaa-1200.webp' });
  expect(await screen.findByText('Foto guardada')).toBeTruthy();
});

test('quitar bien borra el aviso de un error anterior', async () => {
  api.removeImage.mockRejectedValueOnce(new Error('x'));
  const u = userEvent.setup();
  wrap({ imageUrl: 'https://media.test/1/products/7/bbbb-1200.webp' });
  await u.click(screen.getByRole('button', { name: 'Quitar foto' }));
  await u.click(await screen.findByRole('button', { name: 'Quitar' }));
  expect(await screen.findByText(/No se pudo quitar la foto/)).toBeTruthy();
  await u.click(screen.getByRole('button', { name: 'Quitar foto' }));
  await u.click(await screen.findByRole('button', { name: 'Quitar' }));
  await waitFor(() => expect(screen.queryByText(/No se pudo quitar la foto/)).toBeNull());
});

test('sin foto propia pero con la de la categoría no dice «Sin foto» encima de una foto', () => {
  wrap({ categoryImageUrl: 'https://media.test/1/categories/3/cccc-1200.webp' });
  expect(screen.queryByText('Sin foto')).toBeNull();
  expect(screen.getByText('De la categoría')).toBeTruthy();
});

test('«Cambiar» y «Quitar» se leen, no son solo íconos', () => {
  wrap({ imageUrl: 'https://media.test/1/products/7/bbbb-1200.webp' });
  expect(screen.getByRole('button', { name: 'Cambiar foto' })).toHaveTextContent('Cambiar');
  expect(screen.getByRole('button', { name: 'Quitar foto' })).toHaveTextContent('Quitar');
});
