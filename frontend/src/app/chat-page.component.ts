import { CommonModule } from '@angular/common';
import { Component, OnInit } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { AiChatPanelComponent, ChatMessage } from '@berjis/ai-chat';
import { environment } from '../environments/environment';
import { AuthService } from './auth.service';
import { ChatStorageService, ChatSession } from './chat-storage.service';

@Component({
  selector: 'app-chat-page',
  standalone: true,
  imports: [CommonModule, FormsModule, AiChatPanelComponent],
  templateUrl: './chat-page.component.html',
  styleUrls: ['./chat-page.component.css']
})
export class ChatPageComponent implements OnInit {
  env = environment;
  authed = false;
  // fed into the chat panel; replaced on session switch to trigger reseed
  seededMessages: ChatMessage[] = [];
  sessions: ChatSession[] = [];
  currentId: string | null = null;
  mxpx = 0; mypx = 0; fg = '#eff6ff'; bg = '#f8fafc';
  demoInput = '';
  demoPlaceholder = "“Draft a weekly update from my Notes, summarize Docs A & B, and recommend 3 products for Marketplace”";
  year = new Date().getFullYear();
  themeIcon = "🌞";
  appPills = [
    { name: 'Docs', href: 'https://docs.berjis.tech' },
    { name: 'Sheets', href: 'https://sheets.berjis.tech' },
    { name: 'Notes', href: 'https://notes.berjis.tech' },
    { name: 'Slides', href: 'https://slides.berjis.tech' },
    { name: 'PDF', href: 'https://pdf.berjis.tech' },
    { name: 'Logistics', href: 'https://logistics.berjis.tech' },
    { name: 'Marketplace', href: 'https://marketplace.berjis.tech' },
    { name: 'Books', href: 'https://books.berjis.tech' },
    { name: 'Schools', href: 'https://schools.berjis.tech' },
    { name: 'Communities', href: 'https://communities.berjis.tech' },
    { name: 'Cribs', href: 'https://cribs.berjis.tech' },
    { name: 'Architect', href: 'https://architect.berjis.tech' },
    { name: 'Conquer', href: 'https://conquer.berjis.tech' }
  ];
  constructor(private auth: AuthService, private route: ActivatedRoute, private storage: ChatStorageService) {}
  async ngOnInit() {
    this.authed = await this.auth.ensure();
    this.applyThemeColors();
    // Preseed from query params (?q=...&seed=...)
    const qp = this.route.snapshot.queryParamMap;
    const q = (qp.get('q') || '').trim();
    const seed = (qp.get('seed') || '').trim();
    const msgs: ChatMessage[] = [];
    if (q) msgs.push({ role: 'user', content: q });
    if (seed) msgs.push({ role: 'assistant', content: seed });
    // initialize sessions from storage
    this.reloadSessions();
    if (!this.sessions.length) {
      const s = this.storage.create('New Chat', msgs);
      this.sessions = this.storage.list();
      this.currentId = s.id;
    } else {
      // prefer the most recent session
      this.currentId = this.sessions[0].id;
      if (msgs.length) {
        // if query provided seed, create a fresh chat with it
        const s = this.storage.create('New Chat', msgs);
        this.sessions = this.storage.list();
        this.currentId = s.id;
      }
    }
    this.seededMessages = this.current()?.messages || [];
  }
  private reloadSessions() { this.sessions = this.storage.list(); }
  current(): ChatSession | undefined { return this.currentId ? this.storage.get(this.currentId) : undefined; }
  select(id: string) {
    if (this.currentId === id) return;
    this.currentId = id;
    // Replace array reference so AiChatPanel re-seeds via OnChanges
    this.seededMessages = [...(this.current()?.messages || [])];
  }
  newChat() {
    const s = this.storage.create('New Chat', []);
    this.reloadSessions();
    this.select(s.id);
  }
  deleteChat(id: string, ev?: Event) {
    ev?.stopPropagation();
    this.storage.remove(id);
    this.reloadSessions();
    if (this.currentId === id) {
      this.currentId = this.sessions[0]?.id || null;
      this.seededMessages = this.current()?.messages || [];
    }
  }
  private deriveTitle(msgs: ChatMessage[]): string {
    const first = msgs.find(m => m.role === 'user')?.content?.trim() || '';
    const t = first.replace(/\s+/g, ' ').slice(0, 48);
    return t || 'New Chat';
  }
  onMessagesChange(msgs: ChatMessage[]) {
    const now = Date.now();
    const cur = this.current();
    if (!cur) return;
    const title = cur.title === 'New Chat' ? this.deriveTitle(msgs) : cur.title;
    this.storage.upsert({ ...cur, title, updatedAt: now, messages: msgs });
    this.reloadSessions();
  }
  toggleTheme() {
    const el = document.documentElement;
    const next = el.classList.contains('dark') ? 'light' : 'dark';
    el.classList.toggle('dark', next === 'dark');
    localStorage.setItem('theme', next);
    this.applyThemeColors();
  }
  applyThemeColors() {
    const dark = document.documentElement.classList.contains('dark');
    this.fg = dark ? '#0b1437' : '#eff6ff';
    this.bg = dark ? '#0b1324' : '#f8fafc';
    this.themeIcon = dark ? "🌙" : "🌞";
  }
  onMove(e: MouseEvent) {
    const doc = document.documentElement;
    const maxX = Math.max(doc.scrollWidth, window.innerWidth);
    const maxY = Math.max(doc.scrollHeight, window.innerHeight);
    // Use page coordinates so the glow stays under the cursor even when scrolled.
    const x = (e as MouseEvent).pageX;
    const y = (e as MouseEvent).pageY;
    this.mxpx = Math.max(0, Math.min(maxX, x));
    this.mypx = Math.max(0, Math.min(maxY, y));
  }
  goToLogin() {
    const ret = encodeURIComponent(window.location.href);
    window.location.href = `https://berjis.tech/login?returnUrl=${ret}`;
  }
  faqs = [
    { q: 'Is my data private?', a: 'Yes. BerjisAI runs self‑hosted and follows the same auth and security controls as your apps.' },
    { q: 'Do I need to train it?', a: 'No. It works out of the box; you can opt‑in to safe retrieval on public docs later.' },
    { q: 'Does it work across apps?', a: 'Yes. It’s designed to assist across Docs, Sheets, Notes, Marketplace, Logistics, and more.' }
  ];
  openSet = new Set<number>();
  toggleFaq(i: number) { this.openSet.has(i) ? this.openSet.delete(i) : this.openSet.add(i); }
  isOpen(i: number) { return this.openSet.has(i); }
}


