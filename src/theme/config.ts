export const themeConfig = {
  colors: {
    primary: {
      main: '#0091FF',
      light: '#40A9FF',
      dark: '#0066CC',
    },
    secondary: {
      main: '#2A3441',
      light: '#374151',
      dark: '#1E2632',
    },
    success: {
      main: '#00C853',
      light: '#34D399',
      dark: '#00B848',
    },
    error: {
      main: '#FF3B30',
      light: '#FF6B6B',
      dark: '#CC2E26',
    },
    warning: {
      main: '#FF9500',
      light: '#FFB340',
      dark: '#CC7700',
    },
    text: {
      primary: '#353535',
      secondary: '#6B7280',
      disabled: '#9CA3AF',
      dark: '#1A1A1A',
      light: '#FFFFFF',
    },
    background: {
      default: '#F6F8FA',
      paper: '#FFFFFF',
      dark: '#1E2632',
    },
    border: {
      main: '#E5E7EB',
      dark: '#2A3441',
    },
    status: {
      new: '#FF3B30',
      inProgress: '#FF9500',
      completed: '#34C759',
    }
  },
  spacing: {
    xs: '0.25rem',
    sm: '0.5rem',
    md: '1rem',
    lg: '1.5rem',
    xl: '2rem',
  },
  borderRadius: {
    sm: '0.25rem',
    md: '0.5rem',
    lg: '1rem',
    full: '9999px',
  },
  typography: {
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
    fontSizes: {
      xs: '0.75rem',
      sm: '0.875rem',
      md: '1rem',
      lg: '1.125rem',
      xl: '1.25rem',
      '2xl': '1.5rem',
    },
    fontWeights: {
      normal: 400,
      medium: 500,
      semibold: 600,
      bold: 700,
    }
  },
  shadows: {
    sm: '0 1px 2px 0 rgba(0, 0, 0, 0.05)',
    md: '0 4px 6px -1px rgba(0, 0, 0, 0.1)',
    lg: '0 10px 15px -3px rgba(0, 0, 0, 0.1)',
  }
} as const;

export type ThemeConfig = typeof themeConfig; 