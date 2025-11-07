import { CommonModule } from '@angular/common';
import { Component, EventEmitter, Input, OnInit, OnChanges, Output, SimpleChanges } from '@angular/core';
import { Subscription } from 'rxjs';
import { FormsModule } from '@angular/forms';
import { AiChatService, ChatMessage } from '../ai-chat.service';

@Component({
  selector: 'berjis-ai-chat-panel',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './ai-chat-panel.component.html',
  styleUrls: ['./ai-chat-panel.component.css']
})
export class AiChatPanelComponent implements OnInit, OnChanges {
  @Input() base = 'https://ai-api.berjis.tech';
  @Input() model = 'llama3.1:8b';
  @Input() placeholder = 'Ask anything.';
  @Input() presetMessages: ChatMessage[] | null = null;
  @Output() messagesChange = new EventEmitter<ChatMessage[]>();

  // Let the server inject system messages; keep client history user/assistant only
  messages: ChatMessage[] = [];
  input = '';
  sending = false;
  lastError: string | null = null;
  private reqSub: Subscription | null = null;
  themeIcon = '🌞';
  private abort: AbortController | null = null;
  models: string[] = [];
  selectedModel = this.model;

  constructor(private ai: AiChatService) {}

  ngOnInit(): void {
    if (this.presetMessages && this.presetMessages.length) {
      this.messages = this.presetMessages.slice(0, 10);
    }
    const dark = document.documentElement.classList.contains('dark');
    this.themeIcon = dark ? '🌙' : '🌞';
    this.selectedModel = this.model;
    // Fetch available models to allow quick switching
    this.ai.models(this.base).subscribe({
      next: (r: any) => {
        const list: string[] = (r?.data || r?.models || []).map((m: any) => m.name || m).filter((s: any) => typeof s === 'string');
        this.models = list;
      },
      error: () => {}
    });
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['presetMessages'] && !changes['presetMessages'].firstChange) {
      // when parent switches sessions, reseed the panel state
      this.stop();
      const seed: ChatMessage[] = this.presetMessages || [];
      this.messages = seed.slice(0, 10);
    }
  }

  send() {
    const text = this.input.trim();
    if (!text || this.sending) return;
    this.messages.push({ role: 'user', content: text });
    this.input = '';
    this.sending = true;
    const history = this.messages.slice(-10);
    // Pre-append assistant bubble to stream into
    this.messages.push({ role: 'assistant', content: '' });
    const idx = this.messages.length - 1;
    const ac = new AbortController();
    this.abort = ac;
    this.reqSub = this.ai.chatStream(this.base, { model: this.selectedModel || this.model, messages: history, temperature: 0.2 }, ac)
      .subscribe({
        next: (chunk) => {
          if (chunk?.content != null) {
            this.messages[idx].content += this.sanitize(chunk.content);
            this.messagesChange.emit([...this.messages]);
          }
        },
        error: (e: any) => {
          const status = e?.status;
          if (status === 401) {
            this.messages[idx].content = 'Please sign in to use Berjis AI. Open berjis.tech, sign in, then come back.';
          } else if (e?.name === 'AbortError') {
            // interrupted by user
          } else {
            this.messages[idx].content = 'Service is unavailable. Please try again shortly.';
          }
          this.lastError = e?.message || 'unknown error';
          this.sending = false; this.reqSub = null; this.abort = null;
        },
        complete: () => { this.sending = false; this.reqSub = null; this.abort = null; }
      });
  }

  stop() { if (this.abort) { this.abort.abort(); this.abort = null; } if (this.reqSub) { this.reqSub.unsubscribe(); this.reqSub = null; } this.sending = false; }

  toggleTheme() {
    const el = document.documentElement;
    const nextDark = !el.classList.contains('dark');
    el.classList.toggle('dark', nextDark);
    localStorage.setItem('theme', nextDark ? 'dark' : 'light');
    this.themeIcon = nextDark ? '🌙' : '🌞';
  }

  private sanitize(content: string): string {
    const lines = content.split(/\r?\n/).filter(x=>x.trim().length>0);
    if (lines.length >= 10) {
      const avg = lines.reduce((a,b)=>a+b.trim().length,0)/lines.length;
      const oneWordish = lines.filter(l=>l.trim().split(/\s+/).length<=2).length / lines.length > 0.6;
      if (avg < 12 && oneWordish) return lines.join(' ').replace(/\s{2,}/g,' ').trim();
    }
    return content;
  }
}
