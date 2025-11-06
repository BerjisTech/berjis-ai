export const environment = {
  production: false,
  apiBase: (typeof window !== 'undefined' && (window as any).__BERJIS_API__)
    || 'https://api.berjis.tech',
  aiBase: (typeof window !== 'undefined' && (window as any).__BERJIS_AI_API__)
    || 'https://ai-api.berjis.tech'
};
