import React, { createContext, useCallback, useMemo } from 'react';
import { themeConfig, ThemeConfig } from '../theme/config';

interface ThemeContextType {
  theme: ThemeConfig;
  updateTheme: (newTheme: Partial<ThemeConfig>) => void;
}

export const ThemeContext = createContext<ThemeContextType | null>(null);

interface ThemeProviderProps {
  children: React.ReactNode;
  initialTheme?: Partial<ThemeConfig>;
}

export const ThemeProvider: React.FC<ThemeProviderProps> = ({
  children,
  initialTheme = themeConfig,
}) => {
  const [theme, setTheme] = React.useState<ThemeConfig>({
    ...themeConfig,
    ...initialTheme,
  });

  const updateTheme = useCallback((newTheme: Partial<ThemeConfig>) => {
    setTheme((prevTheme) => ({
      ...prevTheme,
      ...newTheme,
    }));
  }, []);

  const value = useMemo(
    () => ({
      theme,
      updateTheme,
    }),
    [theme, updateTheme]
  );

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  );
}; 