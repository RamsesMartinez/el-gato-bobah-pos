import React from 'react';
import { useTheme } from '../../hooks/useTheme';
import { Product } from '../../types/sales';

interface ProductsPanelProps {
  products: Product[];
  selectedCategory: string;
  categories: string[];
  onCategoryChange: (category: string) => void;
  onProductSelect: (product: Product) => void;
}

export const ProductsPanel: React.FC<ProductsPanelProps> = ({
  products,
  selectedCategory,
  categories,
  onCategoryChange,
  onProductSelect,
}) => {
  const { theme } = useTheme();

  return (
    <div style={{
      flex: 1,
      display: 'flex',
      flexDirection: 'column',
      height: '100%',
      backgroundColor: theme.colors.background.default,
    }}>
      {/* Barra de búsqueda y herramientas */}
      <div style={{
        padding: theme.spacing.md,
        display: 'flex',
        gap: theme.spacing.md,
        borderBottom: `1px solid ${theme.colors.border.main}`,
      }}>
        <input
          type="text"
          placeholder="Buscar producto..."
          style={{
            flex: 1,
            padding: theme.spacing.sm,
            borderRadius: theme.borderRadius.md,
            border: `1px solid ${theme.colors.border.main}`,
          }}
        />
        <button style={{
          padding: `${theme.spacing.sm} ${theme.spacing.md}`,
          border: `1px solid ${theme.colors.border.main}`,
          borderRadius: theme.borderRadius.md,
          backgroundColor: theme.colors.background.paper,
        }}>
          <span>📷</span>
        </button>
        <button style={{
          padding: `${theme.spacing.sm} ${theme.spacing.md}`,
          border: `1px solid ${theme.colors.border.main}`,
          borderRadius: theme.borderRadius.md,
          backgroundColor: theme.colors.background.paper,
        }}>
          Promociones
        </button>
      </div>

      {/* Categorías */}
      <div style={{
        padding: theme.spacing.md,
        display: 'flex',
        gap: theme.spacing.md,
        overflowX: 'auto',
      }}>
        {categories.map((category) => (
          <button
            key={category}
            onClick={() => onCategoryChange(category)}
            style={{
              padding: `${theme.spacing.sm} ${theme.spacing.md}`,
              borderRadius: theme.borderRadius.md,
              border: 'none',
              backgroundColor: selectedCategory === category
                ? theme.colors.primary.main
                : theme.colors.background.paper,
              color: selectedCategory === category
                ? theme.colors.background.paper
                : theme.colors.text.primary,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
            }}
          >
            {category}
          </button>
        ))}
      </div>

      {/* Grid de productos */}
      <div style={{
        flex: 1,
        padding: theme.spacing.md,
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))',
        gap: theme.spacing.md,
        overflowY: 'auto',
      }}>
        {products.map((product) => (
          <div
            key={product.id}
            onClick={() => onProductSelect(product)}
            className="product-item"
            style={{
              backgroundColor: theme.colors.background.paper,
              borderRadius: theme.borderRadius.md,
              overflow: 'hidden',
              cursor: 'pointer',
              transition: 'transform 0.2s ease-in-out',
            }}
          >
            <img
              src={product.image_url || '/placeholder-product.png'}
              alt={product.name}
              style={{
                width: '100%',
                height: '150px',
                objectFit: 'cover',
              }}
            />
            <div style={{ padding: theme.spacing.md }}>
              <h3 style={{
                margin: 0,
                fontSize: theme.typography.fontSizes.md,
                fontWeight: theme.typography.fontWeights.medium,
              }}>
                {product.name}
              </h3>
              {product.description && (
                <p style={{
                  margin: `${theme.spacing.xs} 0`,
                  fontSize: theme.typography.fontSizes.sm,
                  color: theme.colors.text.secondary,
                }}>
                  {product.description}
                </p>
              )}
              <p style={{
                margin: `${theme.spacing.xs} 0 0`,
                color: theme.colors.text.secondary,
              }}>
                ${product.price.toFixed(2)}
              </p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}; 