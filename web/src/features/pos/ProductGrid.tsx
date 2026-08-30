import { Grid, Center, Text } from '@chakra-ui/react';
import type { Menu, MenuProduct } from '../../types/pos';
import { ProductTile } from './ProductTile';
import { desglosePrecio, precioDeLista, type ListaActiva } from './precioPlataforma';

interface Props {
  products: MenuProduct[];
  counts: Record<number, number>;
  onTap: (p: MenuProduct) => void;
  showPrice: boolean;
  menu: Menu | undefined;
  lista: ListaActiva;
  onEditPrice?: (p: MenuProduct) => void;
}

export function ProductGrid({ products, counts, onTap, showPrice, menu, lista, onEditPrice }: Props) {
  if (products.length === 0) {
    return (
      <Center h="200px">
        <Text color="fg.muted">No hay productos aquí</Text>
      </Center>
    );
  }
  return (
    <Grid templateColumns="repeat(auto-fill, minmax(150px, 1fr))" gap={2.5} pb={4}>
      {products.map((p) => (
        <ProductTile
          key={p.id}
          product={p}
          count={counts[p.id] ?? 0}
          onTap={onTap}
          showPrice={showPrice}
          price={precioDeLista(menu, lista, p.id, Number(p.price))}
          esManual={desglosePrecio(menu, lista, p.id, Number(p.price))?.esManual ?? false}
          onEditPrice={onEditPrice}
        />
      ))}
    </Grid>
  );
}
