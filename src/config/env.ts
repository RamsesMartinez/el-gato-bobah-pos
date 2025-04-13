interface EnvConfig {
  FUDO_API_URL: string;
  FUDO_API_TOKEN: string;
  DEFAULT_CATEGORY_IMAGE?: string;
}

class MissingEnvironmentError extends Error {
  constructor(envVars: string[]) {
    const message = `
🚨 Error: Variables de entorno faltantes

Las siguientes variables de entorno son requeridas pero no están definidas:
${envVars.map(v => `  - ${v}`).join('\n')}

Por favor:
1. Crea un archivo .env en la raíz del proyecto
2. Copia el contenido de .env.example
3. Completa los valores requeridos

Nota: Si estás viendo este error en desarrollo, asegúrate de que tu archivo .env existe y tiene todas las variables requeridas.
`;
    super(message);
    this.name = 'MissingEnvironmentError';
  }
}

const getEnvConfig = (): EnvConfig => {
  const missingVars: string[] = [];

  // Lista de variables requeridas
  const requiredVars = [
    'REACT_APP_FUDO_API_URL',
    'REACT_APP_FUDO_API_TOKEN'
  ];

  // Verificar todas las variables requeridas
  requiredVars.forEach(varName => {
    if (!process.env[varName]) {
      missingVars.push(varName);
    }
  });

  // Si faltan variables, lanzar error
  if (missingVars.length > 0) {
    throw new MissingEnvironmentError(missingVars);
  }

  // Si llegamos aquí, todas las variables existen
  return {
    FUDO_API_URL: process.env.REACT_APP_FUDO_API_URL!,
    FUDO_API_TOKEN: process.env.REACT_APP_FUDO_API_TOKEN!,
    DEFAULT_CATEGORY_IMAGE: process.env.REACT_APP_DEFAULT_CATEGORY_IMAGE
  };
};

export const env = getEnvConfig(); 