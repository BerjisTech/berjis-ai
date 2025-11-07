export const environment = {
  production: true,
  apiBase: (typeof window !== 'undefined' && (window as any).__BERJIS_API__)
    || 'https://api.berjis.tech',
  aiBase: (typeof window !== 'undefined' && (window as any).__BERJIS_AI_API__)
    || 'https://ai-api.berjis.tech',
  aiModel: (typeof window !== 'undefined' && (window as any).__BERJIS_AI_MODEL__)
    || 'llama3.1:8b'
};
