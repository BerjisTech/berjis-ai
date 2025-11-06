import { Component } from '@angular/core';
import { RouterOutlet } from '@angular/router';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [RouterOutlet],
  template: `
  <div class="min-h-screen bg-[linear-gradient(135deg,#eff6ff,#f8fafc)] dark:bg-[linear-gradient(135deg,#0b1437,#0b1324)]">
    <router-outlet />
  </div>
  `
})
export class AppComponent {}

