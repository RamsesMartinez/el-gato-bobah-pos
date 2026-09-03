import js from '@eslint/js';
import globals from 'globals';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

// LAS FRONTERAS ENTRE CAPAS SON UN ERROR DE BUILD, NO UNA CONVENCIÓN.
//
// Todo el código duplicado que se encontró en este front nació de la misma forma: alguien —persona o
// agente— necesitó algo que ya existía en otra carpeta, y tenía DOS salidas que compilaban:
// importarla cruzado, o copiarla. Se copió. Cuatro veces `round2`, dos veces el armado del cuerpo del
// pedido (con los comentarios copiados palabra por palabra), dos parsers distintos del mismo campo.
// Un comentario que decía "esto se comparte" no lo evitó; una constitución que dice "reutiliza",
// tampoco.
//
// Con estas reglas queda UNA salida y es la correcta: si dos pantallas necesitan lo mismo, baja a
// `domain` (si es regla) o a `components` (si es pintar). Eso deja de ser criterio y pasa a ser lo
// único que compila.
//
// Las capas, de adentro hacia afuera:
//   types/    contratos. No importa de nadie.
//   domain/   reglas puras: dinero, cobro, pedido. Sin React, sin Chakra, sin red, sin estado.
//   api/ stores/ hooks/ utils/ components/   infraestructura y piezas de pantalla.
//   shared/   piezas COMPUESTAS que varias pantallas usan (el ticket, la hoja de cobro). Pueden
//             hablar con la red y con el estado, pero no conocen ninguna pantalla.
//   features/ pantallas. Pueden usar todo lo de arriba; NUNCA otra feature.
//
// `shared/` nació de esta misma limpieza: tres pantallas importaban `features/tickets`, y eso no era
// una pantalla importando a otra — era que el ticket nunca fue una pantalla. Cuando la frontera se
// queja, la pregunta correcta no es "¿cómo la evado?" sino "¿de quién es de verdad esta pieza?".
const FRONTERAS = [
  {
    files: ['src/domain/**/*.ts'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [
          {
            group: ['react', 'react-*', '@chakra-ui/*', '@tanstack/*', 'react-icons*'],
            message:
              'domain es puro: sin React, sin Chakra, sin TanStack. Es lo que deja probar la ' +
              'aritmética del dinero sin montar una pantalla, y por lo que sus tests corren en ms.',
          },
          {
            group: ['**/api/*', '**/stores/*', '**/features/*', '**/components/*', '**/hooks/*'],
            message:
              'domain no conoce a quien lo usa. Si necesitas un tipo de esas capas, el tipo está ' +
              'en el lugar equivocado: muévelo a src/types.',
          },
        ],
      }],
    },
  },
  {
    files: ['src/features/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [
          {
            // Se enumeran las features en vez de usar '../*/*': en minimatch, `*` casa también con
            // `..`, así que ese patrón prohibía '../../api/pos' y todo lo demás que sí es legítimo.
            // La lista explícita además se lee como lo que es — el mapa de pantallas del producto.
            group: [
              '../admin/*', '../auth/*', '../backoffice/*', '../orders/*', '../pos/*',
              '../pwa/*', '../sales/*', '../tickets/*',
              '**/features/admin/*', '**/features/auth/*', '**/features/backoffice/*',
              '**/features/orders/*', '**/features/pos/*', '**/features/pwa/*',
              '**/features/sales/*', '**/features/tickets/*',
            ],
            message:
              'Una pantalla no importa de otra pantalla. Si las dos necesitan lo mismo, bájalo a ' +
              'src/domain (si es una regla) o a src/components (si es pintar). Copiarlo es cómo ' +
              'aparecieron las cuatro versiones de round2 y las dos del armado del pedido.',
          },
        ],
      }],
    },
  },
  {
    files: ['src/shared/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [
          {
            group: ['**/features/*', '../features/*'],
            message:
              'Una pieza compartida no conoce ninguna pantalla. Si la necesita, es de esa pantalla ' +
              'y no va en shared; y si la usan varias, lo que falta es traer también lo que le hace ' +
              'falta.',
          },
        ],
      }],
    },
  },
  {
    files: ['src/components/**/*.{ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [
          {
            group: ['**/features/*', '**/api/*'],
            message:
              'Un componente compartido no sabe de qué pantalla viene ni habla con la red. Recibe ' +
              'lo que necesita por props.',
          },
        ],
      }],
    },
  },
];

// LO QUE NO SE ESCRIBE A MANO, PORQUE YA EXISTE Y CON SU PRUEBA.
//
// Cada patrón de aquí se prohíbe porque ya se reimplementó al menos una vez, y la copia divergió de
// la original sin que nada fallara.
const NADA_DE_REIMPLEMENTAR = {
  // Todo menos `src/domain`, que es donde vive la implementación buena. Dejar fuera `shared/` o
  // `utils/` sería dejar abierta justo la carpeta a la que se muda el código compartido.
  files: [
    'src/features/**/*.{ts,tsx}', 'src/components/**/*.{ts,tsx}', 'src/hooks/**/*.{ts,tsx}',
    'src/shared/**/*.{ts,tsx}', 'src/utils/**/*.{ts,tsx}', 'src/stores/**/*.{ts,tsx}',
    'src/api/**/*.{ts,tsx}', 'src/app/**/*.{ts,tsx}',
  ],
  rules: {
    'no-restricted-syntax': ['error',
      {
        // Math.round(x * 100) / 100 en cualquiera de sus formas.
        selector: 'BinaryExpression[operator="/"][right.value=100] > CallExpression[callee.object.name="Math"][callee.property.name="round"]',
        message:
          'El redondeo a dos decimales es round2() de src/domain/cobro. Escrito a mano son cuatro ' +
          'copias, y dos de ellas se olvidaron del EPSILON que evita que 1.005 baje a 1.00.',
      },
      {
        selector: 'CallExpression[callee.name="parseFloat"]',
        message:
          'Para leer un campo de dinero va parseMonto() de src/domain/cobro. parseFloat("1,000") ' +
          'devuelve 1: la coma de millar que el operador teclea por costumbre cobra $999 de menos, ' +
          'y en una cuenta dividida hace que la suma cuadre por casualidad.',
      },
    ],
  },
};

export default tseslint.config(
  // snippets generados por Chakra CLI + build: no se lintan
  { ignores: ['dist', 'src/components/ui/**'] },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    },
  },
  ...FRONTERAS,
  NADA_DE_REIMPLEMENTAR,
  // Los tests de nodo leen el árbol de archivos: necesitan los globals de node, no los del navegador.
  {
    files: ['src/**/*.test.{ts,tsx}'],
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
  },
);
