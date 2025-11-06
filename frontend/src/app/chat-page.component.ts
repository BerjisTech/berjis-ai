import { CommonModule } from '@angular/common';
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AiChatPanelComponent } from '@berjis/ai-chat';
import { environment } from '../environments/environment';
import { AuthService } from './auth.service';

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
  mx = 50; my = 50; fg = '#eff6ff'; bg = '#f8fafc';
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
  constructor(private auth: AuthService) {}
  async ngOnInit() {
    this.authed = await this.auth.ensure();
    this.applyThemeColors();
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
    this.mx = Math.max(0, Math.min(100, (e.clientX / window.innerWidth) * 100));
    this.my = Math.max(0, Math.min(100, (e.clientY / window.innerHeight) * 100));
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


