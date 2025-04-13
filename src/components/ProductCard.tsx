import React from 'react';
import { useTheme } from '../hooks/useTheme';

interface ProductCardProps {
  name: string;
  price: number;
  image: string;
  description?: string;
  onAdd?: () => void;
}

export const ProductCard: React.FC<ProductCardProps> = ({
  name,
  price,
  image,
  description,
  onAdd,
}) => {
  const { theme } = useTheme();

  return (
    <div className="product-card">
      <img 
        src={image} 
        alt={name} 
        style={{ 
          width: '100%', 
          height: '200px', 
          objectFit: 'cover' 
        }} 
      />
      <div style={{ padding: theme.spacing.md }}>
        <h3 style={{ 
          fontSize: theme.typography.fontSizes.lg,
          fontWeight: theme.typography.fontWeights.semibold,
          marginBottom: theme.spacing.xs
        }}>
          {name}
        </h3>
        {description && (
          <p style={{ 
            color: theme.colors.text.secondary,
            fontSize: theme.typography.fontSizes.sm,
            marginBottom: theme.spacing.sm
          }}>
            {description}
          </p>
        )}
        <div style={{ 
          display: 'flex', 
          justifyContent: 'space-between',
          alignItems: 'center',
          marginTop: theme.spacing.sm
        }}>
          <span style={{ 
            fontSize: theme.typography.fontSizes.xl,
            fontWeight: theme.typography.fontWeights.bold,
            color: theme.colors.primary.main
          }}>
            ${price.toFixed(2)}
          </span>
          {onAdd && (
            <button 
              className="button button-primary"
              onClick={onAdd}
              style={{
                backgroundColor: theme.colors.primary.main,
                color: theme.colors.background.paper,
                border: 'none',
                padding: `${theme.spacing.sm} ${theme.spacing.md}`,
                borderRadius: theme.borderRadius.md,
                cursor: 'pointer',
                transition: 'background-color 0.2s ease-in-out'
              }}
            >
              Agregar
            </button>
          )}
        </div>
      </div>
    </div>
  );
}; 