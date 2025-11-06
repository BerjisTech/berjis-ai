import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';
import { environment } from '../environments/environment';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);
  private base = environment.apiBase;

  verify() {
    return firstValueFrom(this.http.post<{ success: boolean; data?: { valid: boolean } }>(`${this.base}/v1/auth/verify`, {}, { withCredentials: true }));
  }
  refresh() {
    return firstValueFrom(this.http.post(`${this.base}/v1/auth/refresh`, {}, { withCredentials: true }));
  }
  async ensure(): Promise<boolean> {
    try {
      const v = await this.verify();
      if (v?.data?.valid) return true;
      await this.refresh();
      const v2 = await this.verify();
      return !!v2?.data?.valid;
    } catch {
      try {
        await this.refresh();
        const v2 = await this.verify();
        return !!v2?.data?.valid;
      } catch { return false; }
    }
  }
}

