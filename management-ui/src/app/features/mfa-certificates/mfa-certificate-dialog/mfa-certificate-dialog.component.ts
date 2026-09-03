import { Component, OnInit, Inject, OnDestroy } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Observable, Subject } from 'rxjs';
import { CertModel } from '../../../shared/models/cert-model';
import { MfaCertificatesService } from '../mfa-certificates.service';
import { AppUtilsService } from '../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { MfaCaModel } from '../../../shared/models/mfa-ca-model';
import { MfaCrlModel } from '../../../shared/models/mfa-crl-model';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-mfa-certificate-dialog',
    templateUrl: './mfa-certificate-dialog.component.html',
    styleUrls: ['./mfa-certificate-dialog.component.scss'],
    standalone: false,
})
export class MfaCertificateDialogComponent extends CommonFormDialogComponent implements OnDestroy, OnInit {
    form: FormGroup;
    protected componentDestroyed: Subject<void> = new Subject<void>();

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<MfaCertificateDialogComponent>,
        protected fb: FormBuilder,
        protected mfaCertificatesService: MfaCertificatesService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService

    ) {
        super(dialogRef, appUtilsService, translateService);
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            mfaCertificate: ['', Validators.required],
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    submit() {
        let certObj = new CertModel({ data: this.form.value.mfaCertificate });
        let action1 = this.mfaCertificatesService.addMfaCertificateCrl(certObj);
        let action2: Observable<MfaCaModel[]> | Observable<MfaCrlModel[]> = this.mfaCertificatesService.loadMfaCertificatesCrl();

        if (this.data.certificateType === 'CA') {
            action1 = this.mfaCertificatesService.addMfaCertificateCa(certObj);
            action2 = this.mfaCertificatesService.loadMfaCertificatesCa();
        }

        this.performSavingAction(action1, action2);
    }

    closeDialog() {
        this.dialogRef.close();
    }

}
