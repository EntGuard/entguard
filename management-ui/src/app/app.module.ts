import { BrowserModule } from '@angular/platform-browser';
import { NgModule } from '@angular/core';
import { RouterModule } from '@angular/router';

import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';
import { BrowserAnimationsModule } from '@angular/platform-browser/animations';
import { HeaderComponent } from './core/header/header.component';
import { HTTP_INTERCEPTORS, HttpClientModule } from '@angular/common/http';
import { LoginComponent } from './features/login/login.component';
import { ReactiveFormsModule } from '@angular/forms';
import { ConfirmDialogComponent } from './shared/dialogs/delete-dialog/confirm-dialog.component';
import { AppAgGridModule } from './shared/components/ag-grid/app-ag-grid.module';
import { AddressPoolsModule } from './features/address-pools/address-pools.module';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatDialogModule } from '@angular/material/dialog';
import { SharedModule } from './shared/shared.module';
import { AgGridModule } from 'ag-grid-angular';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { JwtInterceptor } from './core/jwt.interceptor';
import { ErrorInterceptor } from './core/error.interceptor';
import { MatChipsModule } from '@angular/material/chips';
import { MatSelectModule } from '@angular/material/select';
import {
    MatTooltipModule,
    MAT_TOOLTIP_DEFAULT_OPTIONS,
    MatTooltipDefaultOptions,
} from '@angular/material/tooltip';
import { MAT_AUTOCOMPLETE_SCROLL_STRATEGY, MatAutocompleteModule } from '@angular/material/autocomplete';
import { Overlay, BlockScrollStrategy } from '@angular/cdk/overlay';
import { PremiumFeatureComponent } from './features/unsupported-feature/premium-feature.component';
import { TranslateModule } from '@ngx-translate/core';
import { provideTranslateHttpLoader } from '@ngx-translate/http-loader';
import { HttpErrorDialogComponent } from './shared/dialogs/http-error-dialog/http-error-dialog.component';
import { MatSnackBarModule } from '@angular/material/snack-bar';
import { MatProgressBarModule } from '@angular/material/progress-bar';

export function scrollFactory(overlay: Overlay): () => BlockScrollStrategy {
    return () => overlay.scrollStrategies.block();
}

const tooltipDefaults: MatTooltipDefaultOptions = {
    showDelay: 0,
    hideDelay: 0,
    touchendHideDelay: 1500,
    disableTooltipInteractivity: true,
};

@NgModule({
    declarations: [
        AppComponent,
        HeaderComponent,
        LoginComponent,
        PremiumFeatureComponent,
        ConfirmDialogComponent,
        HttpErrorDialogComponent,
    ],
    imports: [
        BrowserModule,
        HttpClientModule,
        RouterModule,
        AppRoutingModule,
        BrowserAnimationsModule,
        ReactiveFormsModule,
        MatIconModule,
        MatInputModule,
        MatButtonModule,
        MatDialogModule,
        SharedModule,
        AgGridModule,
        AppAgGridModule,
        MatProgressSpinnerModule,
        MatChipsModule,
        MatSelectModule,
        MatTooltipModule,
        MatSnackBarModule,
        MatProgressBarModule,
        AddressPoolsModule,
        TranslateModule.forRoot({
            loader: provideTranslateHttpLoader({
                prefix: '/assets/i18n/',
                suffix: '.json'
            }),
            fallbackLang: 'en'
        })
    ],
    providers: [
        { provide: HTTP_INTERCEPTORS, useClass: JwtInterceptor, multi: true },
        { provide: HTTP_INTERCEPTORS, useClass: ErrorInterceptor, multi: true },
        {
            provide: MAT_AUTOCOMPLETE_SCROLL_STRATEGY,
            useFactory: scrollFactory,
            deps: [Overlay],
        },
        {
            provide: MAT_TOOLTIP_DEFAULT_OPTIONS,
            useValue: tooltipDefaults,
        },
    ],
    bootstrap: [AppComponent],
})
export class AppModule { }
