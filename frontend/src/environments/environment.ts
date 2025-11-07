const w: any = typeof window !== 'undefined' ? (window as any) : {};
const host = typeof window !== 'undefined' ? window.location.hostname : '';
const isLocal = host === 'localhost' || host === '127.0.0.1';

export const environment = {
  production: false,
  apiBase: w.__BERJIS_API__ || (isLocal ? 'http://localhost:8080' : 'https://api.berjis.tech'),
  aiBase: w.__BERJIS_AI_API__ || (isLocal ? 'http://localhost:8094' : 'https://ai-api.berjis.tech'),
  aiModel: w.__BERJIS_AI_MODEL__ || 'llama3.1:8b'
};
