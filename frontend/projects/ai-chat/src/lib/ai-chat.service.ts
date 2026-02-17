import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

export type ChatMessage = { role: 'system' | 'user' | 'assistant', content: string };

@Injectable({ providedIn: 'root' })
export class AiChatService {
  private http = inject(HttpClient);
  constructor() {}

  chat(base: string, req: { model: string; messages: ChatMessage[]; temperature?: number }) {
    return this.http.post<any>(`${base.replace(/\/$/,'')}/v1/chat`, req, { withCredentials: true });
  }
  models(base: string) {
    return this.http.get<any>(`${base.replace(/\/$/,'')}/v1/models`, { withCredentials: true });
  }

  // RAG endpoints
  ingestDocument(base: string, payload: { title: string; source?: string; content: string }) {
    return this.http.post<any>(`${base.replace(/\/$/,'')}/v1/documents`, payload, { withCredentials: true });
  }

  listDocuments(base: string) {
    return this.http.get<any>(`${base.replace(/\/$/,'')}/v1/documents`, { withCredentials: true });
  }

  deleteDocument(base: string, id: string) {
    return this.http.delete<any>(`${base.replace(/\/$/,'')}/v1/documents/${encodeURIComponent(id)}`, { withCredentials: true });
  }

  ragSearch(base: string, query: string, topK = 5) {
    return this.http.post<any>(`${base.replace(/\/$/,'')}/v1/search`, { query, topK }, { withCredentials: true });
  }

  ragChat(base: string, payload: { model: string; question: string; topK?: number }) {
    return this.http.post<any>(`${base.replace(/\/$/,'')}/v1/chat/rag`, payload, { withCredentials: true });
  }

  chatStream(base: string, req: { model: string; messages: ChatMessage[]; temperature?: number }, abort: AbortController): Observable<{ role: 'assistant'|'user'|'system'; content: string; done?: boolean }> {
    const url = `${base.replace(/\/$/,'')}/v1/chat/stream`;
    return new Observable(observer => {
      (async () => {
        try {
          const res = await fetch(url, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/x-ndjson' },
            body: JSON.stringify(req),
            signal: abort.signal,
          });
          if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';
          while (true) {
            const { value, done } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            let idx;
            while ((idx = buffer.indexOf('\n')) >= 0) {
              let line = buffer.slice(0, idx).trim();
              buffer = buffer.slice(idx + 1);
              if (!line) continue;
              // Support both NDJSON and SSE ('data: {...}')
              if (line.startsWith('data:')) line = line.slice(5).trim();
              try {
                const obj = JSON.parse(line);
                const msg = obj?.message;
                if (msg?.content != null) {
                  observer.next({ role: msg.role, content: msg.content, done: !!obj?.done });
                }
                if (obj?.done) {
                  observer.complete();
                }
              } catch {}
            }
          }
          observer.complete();
        } catch (e: any) {
          if (e?.name === 'AbortError') {
            observer.complete();
          } else {
            observer.error(e);
          }
        }
      })();
      return () => abort.abort();
    });
  }
}
