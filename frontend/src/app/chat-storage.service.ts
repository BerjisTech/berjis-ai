import { Injectable } from '@angular/core';
import { ChatMessage } from '@berjis/ai-chat';

export type ChatSession = {
  id: string;
  title: string;
  createdAt: number;
  updatedAt: number;
  messages: ChatMessage[];
};

@Injectable({ providedIn: 'root' })
export class ChatStorageService {
  private key = 'berjis_ai_sessions';

  private loadAll(): ChatSession[] {
    try { return JSON.parse(localStorage.getItem(this.key) || '[]'); } catch { return []; }
  }
  private saveAll(list: ChatSession[]) { localStorage.setItem(this.key, JSON.stringify(list)); }

  list(): ChatSession[] { return this.loadAll().sort((a,b)=>b.updatedAt-a.updatedAt); }
  get(id: string): ChatSession | undefined { return this.loadAll().find(s=>s.id===id); }

  upsert(sess: ChatSession) {
    const list = this.loadAll();
    const i = list.findIndex(s=>s.id===sess.id);
    if (i>=0) list[i] = sess; else list.push(sess);
    this.saveAll(list);
  }
  create(title: string, messages: ChatMessage[]): ChatSession {
    const now = Date.now();
    const s: ChatSession = { id: crypto.randomUUID(), title, createdAt: now, updatedAt: now, messages };
    this.upsert(s); return s;
  }
  remove(id: string) { this.saveAll(this.loadAll().filter(s=>s.id!==id)); }
}

