import { Injectable, inject } from '@angular/core';
import { CoreAuthService } from '@berjis/angular-auth';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private core = inject(CoreAuthService);

  async ensure(): Promise<boolean> {
    const session = await this.core.ensureAuth({ maxAgeMs: 1500 });
    return !!session?.valid;
  }

  get session() {
    return this.core.getSession();
  }
}

