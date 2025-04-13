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

  // Función para generar un color pastel aleatorio pero consistente para cada categoría
  const getCategoryColor = (categoryName: string) => {
    const colors = [
      '#FFB5E8', // Rosa pastel
      '#B5B9FF', // Azul pastel
      '#B5FFB9', // Verde pastel
      '#FFB8B5', // Coral pastel
      '#BFFCC6', // Menta pastel
      '#FFC9DE', // Rosa claro
      '#C4FAF8', // Turquesa pastel
      '#DBCDF0', // Lavanda pastel
    ];
    
    // Usar el nombre de la categoría para seleccionar un color consistente
    const index = categoryName.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0);
    return colors[index % colors.length];
  };

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

      {/* Categorías en cuadrícula */}
      <div style={{
        padding: theme.spacing.md,
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(150px, 1fr))',
        gap: theme.spacing.md,
        maxHeight: '200px',
        overflowY: 'auto',
        borderBottom: `1px solid ${theme.colors.border.main}`,
      }}>
        {categories.map((category) => {
          const isSelected = selectedCategory === category;
          const backgroundColor = getCategoryColor(category);
          
          return (
            <button
              key={category}
              onClick={() => onCategoryChange(category)}
              style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                padding: theme.spacing.md,
                borderRadius: theme.borderRadius.lg,
                border: isSelected ? `2px solid ${theme.colors.primary.main}` : 'none',
                backgroundColor: backgroundColor,
                color: theme.colors.text.primary,
                cursor: 'pointer',
                minHeight: '100px',
                transition: 'all 0.2s ease-in-out',
                transform: isSelected ? 'scale(0.98)' : 'scale(1)',
                boxShadow: isSelected 
                  ? 'inset 0 2px 4px rgba(0,0,0,0.1)' 
                  : '0 2px 4px rgba(0,0,0,0.05)',
              }}
            >
              <div style={{
                fontSize: '24px',
                marginBottom: theme.spacing.sm,
              }}>
                {/* Emoji basado en la categoría */}
                {category === 'Todos los productos' ? '🏪' :
                 category.toLowerCase().includes('frappe') ? '🥤' :
                 category.toLowerCase().includes('té') ? '🫖' :
                 category.toLowerCase().includes('café') ? '☕' :
                 category.toLowerCase().includes('chocolate') ? '🍫' :
                 '🍽️'}
              </div>
              <span style={{
                fontSize: theme.typography.fontSizes.sm,
                fontWeight: theme.typography.fontWeights.medium,
                textAlign: 'center',
                wordBreak: 'break-word',
              }}>
                {category}
              </span>
            </button>
          );
        })}
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