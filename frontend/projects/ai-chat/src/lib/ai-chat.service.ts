import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';

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
}

