import { CommonModule } from '@angular/common';
import { Component, Input } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AiChatService, ChatMessage } from '../ai-chat.service';

@Component({
  selector: 'berjis-ai-chat-panel',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './ai-chat-panel.component.html',
  styleUrls: ['./ai-chat-panel.component.css']
})
export class AiChatPanelComponent {
  @Input() base = 'https://ai-api.berjis.tech';
  @Input() model = 'llama3.1:8b';
  @Input() placeholder = 'Ask anything…';

  messages: ChatMessage[] = [
    { role: 'system', content: 'You are a helpful assistant for the Berjis ecosystem.' }
  ];
  input = '';
  sending = false;
  lastError: string | null = null;

  constructor(private ai: AiChatService) {}

  async send() {
    const text = this.input.trim();
    if (!text || this.sending) return;
    this.messages.push({ role: 'user', content: text });
    this.input = '';
    this.sending = true;
    try {
      const resp: any = await this.ai.chat(this.base, { model: this.model, messages: this.messages }).toPromise();
      const m: ChatMessage = resp?.data?.message || resp?.message;
      if (m?.content) this.messages.push({ role: 'assistant', content: m.content });
      this.lastError = null;
    } catch (e: any) {
      const status = e?.status;
      if (status === 401) {
        this.messages.push({ role: 'assistant', content: 'Please sign in to use Berjis AI. Open berjis.tech, sign in, then come back.' });
      } else {
        this.messages.push({ role: 'assistant', content: 'Service is unavailable. Please try again shortly.' });
      }
      this.lastError = e?.message || 'unknown error';
    } finally {
      this.sending = false;
    }
  }
}
