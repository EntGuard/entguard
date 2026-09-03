import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { MfaCertificatesComponent } from "./mfa-certificates.component"
import { MfaCertificatesRoutingModule } from './mfa-certificates-routing.module';

import { MatIconModule } from '@angular/material/icon';
import { MatButtonModule } from '@angular/material/button';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { AppAgGridModule } from '../../shared/components/ag-grid/app-ag-grid.module';
import { ReactiveFormsModule } from '@angular/forms';
import { MfaCertificateDialogComponent } from './mfa-certificate-dialog/mfa-certificate-dialog.component';
import { MfaCertificatesIssuerDialogComponent } from './mfa-certificates-issuer-dialog/mfa-certificates-issuer-dialog.component';
import { TranslateModule } from "@ngx-translate/core";

@NgModule({
    declarations: [MfaCertificatesComponent, MfaCertificateDialogComponent, MfaCertificatesIssuerDialogComponent],
    imports: [
        CommonModule,
        MfaCertificatesRoutingModule,
        MatIconModule,
        MatButtonModule,
        MatTooltipModule,
        MatInputModule,
        MatProgressSpinnerModule,
        MatProgressBarModule,
        AppAgGridModule,
        ReactiveFormsModule,
        TranslateModule.forChild(),
    ]
})
export class MfaCertificatesModule { }
