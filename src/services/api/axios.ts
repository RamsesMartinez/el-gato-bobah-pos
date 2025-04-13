import axios from 'axios';
import { env } from '../../config/env';

// Configuración base de axios
const api = axios.create({
  baseURL: env.FUDO_API_URL,
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${env.FUDO_API_TOKEN}`,
  },
});

// Interceptor para manejar errores
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response) {
      // Error de respuesta del servidor
      console.error('API Error:', error.response.data);
      if (error.response.status === 401) {
        // Token inválido o expirado
        console.error('Token inválido o expirado');
        // Aquí podrías implementar un refresh token o redireccionar al login
      }
    }
    return Promise.reject(error);
  }
);

export default api; 