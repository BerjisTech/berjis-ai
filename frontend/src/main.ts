import { bootstrapApplication } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { Routes, provideRouter } from '@angular/router';
import { AppComponent } from './app/app.component';
import { ChatPageComponent } from './app/chat-page.component';
import { CORE_AUTH_API_BASE } from '@berjis/angular-auth';
import { environment } from './environments/environment';

const routes: Routes = [
  { path: '', component: ChatPageComponent }
];

bootstrapApplication(AppComponent, {
  providers: [
    provideHttpClient(),
    provideRouter(routes),
    { provide: CORE_AUTH_API_BASE, useValue: environment.apiBase }
  ]
}).catch(err => console.error(err));
