import { bootstrapApplication } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { Routes, provideRouter } from '@angular/router';
import { AppComponent } from './app/app.component';
import { ChatPageComponent } from './app/chat-page.component';

const routes: Routes = [
  { path: '', component: ChatPageComponent }
];

bootstrapApplication(AppComponent, {
  providers: [provideHttpClient(), provideRouter(routes)]
}).catch(err => console.error(err));

