import { enableProdMode, provideZoneChangeDetection } from '@angular/core';
import { platformBrowserDynamic } from '@angular/platform-browser-dynamic';
import { ModuleRegistry, AllCommunityModule } from 'ag-grid-community';

import { AppModule } from './app/app.module';
import { environment } from './environments/environment';

// Register AG Grid modules
ModuleRegistry.registerModules([AllCommunityModule]);

if (environment.production) {
    enableProdMode();
}

platformBrowserDynamic().bootstrapModule(AppModule, { applicationProviders: [provideZoneChangeDetection()], })
    .catch(err => console.error(err));
