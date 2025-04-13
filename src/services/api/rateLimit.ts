import axios, { AxiosError } from 'axios';

class RateLimiter {
  private queue: Array<() => Promise<any>> = [];
  private processing = false;
  private lastRequestTime = 0;
  private minRequestInterval = 1000; // 1 segundo entre solicitudes
  private maxRetries = 3;
  private retryDelay = 2000; // 2 segundos entre reintentos

  async enqueue<T>(request: () => Promise<T>): Promise<T> {
    return new Promise((resolve, reject) => {
      this.queue.push(async () => {
        try {
          const result = await this.executeWithRetry(request);
          resolve(result);
        } catch (error) {
          reject(error);
        }
      });

      if (!this.processing) {
        this.processQueue();
      }
    });
  }

  private async executeWithRetry<T>(
    request: () => Promise<T>,
    retryCount = 0
  ): Promise<T> {
    try {
      const timeSinceLastRequest = Date.now() - this.lastRequestTime;
      if (timeSinceLastRequest < this.minRequestInterval) {
        await new Promise(resolve => 
          setTimeout(resolve, this.minRequestInterval - timeSinceLastRequest)
        );
      }

      this.lastRequestTime = Date.now();
      return await request();
    } catch (error) {
      if (axios.isAxiosError(error)) {
        const axiosError = error as AxiosError;
        
        // Si es error 429, intentar de nuevo
        if (axiosError.response?.status === 429 && retryCount < this.maxRetries) {
          console.warn(`Rate limit alcanzado, reintentando en ${this.retryDelay/1000} segundos...`);
          await new Promise(resolve => setTimeout(resolve, this.retryDelay));
          return this.executeWithRetry(request, retryCount + 1);
        }
      }
      throw error;
    }
  }

  private async processQueue() {
    this.processing = true;
    
    while (this.queue.length > 0) {
      const request = this.queue.shift();
      if (request) {
        await request();
      }
    }

    this.processing = false;
  }
}

export const rateLimiter = new RateLimiter(); 