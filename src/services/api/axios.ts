import axios from 'axios';

// Configuración base de axios
const api = axios.create({
  baseURL: process.env.REACT_APP_API_URL || 'https://api.staging.fu.do/v1alpha1',
  headers: {
    'Content-Type': 'application/json',
  },
});

// Interceptor para manejar errores
api.interceptors.response.use(
  (response) => response,
  (error) => {
    // Aquí podemos manejar errores globales
    console.error('API Error:', error);
    return Promise.reject(error);
  }
);

export default api; 