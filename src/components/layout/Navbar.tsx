import React from 'react';
import { useTheme } from '../../hooks/useTheme';

interface NavbarProps {
  userName: string;
  tableNumber: string;
  onBackClick: () => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  userName,
  tableNumber,
  onBackClick,
}) => {
  const { theme } = useTheme();

  return (
    <nav style={{
      backgroundColor: theme.colors.background.dark,
      padding: theme.spacing.md,
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      color: theme.colors.background.paper,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: theme.spacing.md }}>
        <button
          onClick={onBackClick}
          style={{
            background: 'none',
            border: 'none',
            color: theme.colors.background.paper,
            cursor: 'pointer',
            padding: theme.spacing.sm,
            display: 'flex',
            alignItems: 'center',
            gap: theme.spacing.xs,
          }}
        >
          ← Órdenes
        </button>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: theme.spacing.sm,
        }}>
          <span style={{
            padding: `${theme.spacing.xs} ${theme.spacing.sm}`,
            backgroundColor: theme.colors.primary.main,
            borderRadius: theme.borderRadius.sm,
          }}>Recibo</span>
          <span>Cliente</span>
        </div>
      </div>

      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: theme.spacing.md,
      }}>
        <div style={{ textAlign: 'center' }}>
          <div>Ticket Nº{tableNumber}</div>
          <div style={{ 
            fontSize: theme.typography.fontSizes.sm,
            color: theme.colors.text.disabled 
          }}>
            Mesa {tableNumber}
          </div>
        </div>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: theme.spacing.sm,
        }}>
          <span>{userName}</span>
          <span>🔒</span>
        </div>
      </div>
    </nav>
  );
}; 