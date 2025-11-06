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
  template: `
  <section class="px-6 md:px-16 py-8 md:py-12" *ngIf="authed; else marketing">
    <header class="flex items-center justify-between mb-6">
      <h1 class="text-2xl md:text-3xl font-bold text-slate-900 dark:text-white">Berjis AI</h1>
      <button class="text-sm text-slate-700 dark:text-slate-200" (click)="toggleTheme()">Toggle Theme</button>
    </header>
    <div class="grid md:grid-cols-[1fr_min(460px,40vw)] gap-6">
      <div class="rounded-xl p-6 border border-blue-900/20 dark:border-slate-800 bg-white/80 dark:bg-slate-900/70">
        <h2 class="text-lg font-semibold mb-2 text-slate-900 dark:text-white">What to expect</h2>
        <ul class="list-disc pl-5 text-slate-700 dark:text-slate-200 text-sm">
          <li>Private, self-hosted AI across the Berjis ecosystem.</li>
          <li>Best for summaries, writing help, planning, and Q&A.</li>
          <li>Use the chat on the right to start a conversation.</li>
        </ul>
      </div>
      <div class="min-h-[60vh]">
        <berjis-ai-chat-panel [base]="env.aiBase"></berjis-ai-chat-panel>
      </div>
    </div>
  </section>

  <ng-template #marketing>
    <section class="hero-landing" (mousemove)="onMove($event)" [style.--mx]="mx + '%'" [style.--my]="my + '%'" [style.--fg]="fg" [style.--bg]="bg">
      <div class="nav">
        <div class="brand">Berjis<span class="accent">AI</span></div>
        <div class="actions">
          <button class="btn toggle" (click)="toggleTheme()" aria-label="Toggle theme">{{ themeIcon }}</button>
          <a class="btn ghost" (click)="goToLogin()">Login</a>
          <a class="btn primary" (click)="goToLogin()">Try Now</a>
        </div>
      </div>
      <div class="center">
        <h2 class="tagline">Get the most out of your <span class="gold">Berjis Ecosystem</span></h2>
        <p class="sub">Your private, self‑hosted assistant that works across Docs, Sheets, Notes, Marketplace, Logistics, and more.</p>
        <div class="prompt">
          <div class="icons"><span title="Attach files">📎</span><span title="Tag apps">🏷️</span><span title="Magic prompt">✨</span></div>
          <input [(ngModel)]="demoInput" (keyup.enter)="goToLogin()" [placeholder]="demoPlaceholder" />
        </div>
        <div class="pills">
          <a *ngFor="let app of appPills" class="pill" [href]="app.href" target="_blank" rel="noopener">{{ app.name }}</a>
        </div>
      </div>
    </section>

    <section class="capabilities">
      <div class="grid">
        <article class="tile lg">
          <h3>Summarize & explain</h3>
          <p>Turn long docs, notes, or PDFs into clear summaries and next steps.</p>
        </article>
        <article class="tile sm">
          <h3>Write & polish</h3>
          <p>Draft emails, proposals, and posts in your tone.</p>
        </article>
        <article class="tile md">
          <h3>Plan & schedule</h3>
          <p>Create task lists, timelines, and reminders across apps.</p>
        </article>
        <article class="tile md">
          <h3>Analyze & recommend</h3>
          <p>Explore shop performance and inventory trends.</p>
        </article>
        <article class="tile sm">
          <h3>Help build</h3>
          <p>Suggest components and patterns in Architect.</p>
        </article>
      </div>
    </section>

    <section class="faq">
      <h3>Frequently asked questions</h3>
      <div class="accordion">
        <div class="item" *ngFor="let f of faqs; let i = index">
          <button class="q" (click)="toggleFaq(i)">
            <span class="icon">{{ isOpen(i) ? '−' : '+' }}</span>
            <span class="text">{{ f.q }}</span>
          </button>
          <div class="a" [class.open]="isOpen(i)">
            <p>{{ f.a }}</p>
          </div>
        </div>
      </div>
    </section>

    <section class="cta">
      <h3>Ready to get more from Berjis?</h3>
      <button class="btn primary" (click)="goToLogin()">Sign in to Berjis</button>
    </section>

    <footer class="footer">
      <span>© {{year}} Berjis</span>
      <a href="https://berjis.tech" target="_blank" rel="noopener">berjis.tech</a>
    </footer>
  </ng-template>
  `,
  styleUrls: ['./chat-page.component.css']
})
export class ChatPageComponent implements OnInit {
  env = environment;
  authed = false;
  mx = 50; my = 50; fg = '#eff6ff'; bg = '#f8fafc';
  demoInput = '';
  demoPlaceholder = '“Draft a weekly update from my Notes, summarize Docs A & B, and recommend 3 products for Marketplace”';
  year = new Date().getFullYear();
  themeIcon = '🌞';
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
    this.themeIcon = dark ? '🌙' : '🌞';
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
