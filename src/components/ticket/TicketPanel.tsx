import React from 'react';
import { useTheme } from '../../hooks/useTheme';

interface OrderItem {
  id: string;
  name: string;
  quantity: number;
  price: number;
  total: number;
  details?: string;
}

interface TicketPanelProps {
  items: OrderItem[];
  onAddGuest: () => void;
  onPay: () => void;
}

export const TicketPanel: React.FC<TicketPanelProps> = ({
  items,
  onAddGuest,
  onPay,
}) => {
  const { theme } = useTheme();
  const total = items.reduce((sum, item) => sum + item.total, 0);

  return (
    <div style={{
      backgroundColor: theme.colors.background.paper,
      borderRight: `1px solid ${theme.colors.border.main}`,
      height: '100%',
      width: '400px',
      display: 'flex',
      flexDirection: 'column',
    }}>
      <div style={{ flex: 1, overflowY: 'auto', padding: theme.spacing.md }}>
        {items.map((item) => (
          <div
            key={item.id}
            style={{
              marginBottom: theme.spacing.md,
              padding: theme.spacing.md,
              backgroundColor: theme.colors.background.default,
              borderRadius: theme.borderRadius.md,
            }}
          >
            <div style={{
              display: 'flex',
              justifyContent: 'space-between',
              marginBottom: theme.spacing.xs,
            }}>
              <span style={{ fontWeight: theme.typography.fontWeights.medium }}>
                {item.name}
              </span>
              <span>{item.total.toFixed(2)}</span>
            </div>
            {item.details && (
              <div style={{
                fontSize: theme.typography.fontSizes.sm,
                color: theme.colors.text.secondary,
              }}>
                {item.details}
              </div>
            )}
            <div style={{
              display: 'flex',
              justifyContent: 'space-between',
              marginTop: theme.spacing.xs,
              fontSize: theme.typography.fontSizes.sm,
            }}>
              <span>Cantidad: {item.quantity}</span>
              <span>Precio: {item.price.toFixed(2)}</span>
            </div>
          </div>
        ))}
      </div>

      <button
        onClick={onAddGuest}
        style={{
          padding: theme.spacing.md,
          backgroundColor: 'transparent',
          border: 'none',
          borderTop: `1px solid ${theme.colors.border.main}`,
          color: theme.colors.primary.main,
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          gap: theme.spacing.sm,
        }}
      >
        <span>👤</span>
        <span>AÑADIR COMENSAL</span>
      </button>

      <div style={{
        padding: theme.spacing.md,
        borderTop: `1px solid ${theme.colors.border.main}`,
      }}>
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: theme.spacing.md,
        }}>
          <span style={{ fontWeight: theme.typography.fontWeights.semibold }}>
            Precio total
          </span>
          <span style={{ fontWeight: theme.typography.fontWeights.bold }}>
            ${total.toFixed(2)}
          </span>
        </div>
        <button
          onClick={onPay}
          style={{
            width: '100%',
            padding: theme.spacing.md,
            backgroundColor: theme.colors.success.main,
            color: theme.colors.background.paper,
            border: 'none',
            borderRadius: theme.borderRadius.md,
            fontWeight: theme.typography.fontWeights.medium,
            cursor: 'pointer',
          }}
        >
          Pagar
        </button>
      </div>
    </div>
  );
}; 