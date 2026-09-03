import { Component, OnInit, Inject } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

@Component({
    selector: 'app-mfa-certificates-issuer-dialog',
    templateUrl: './mfa-certificates-issuer-dialog.component.html',
    styleUrls: ['./mfa-certificates-issuer-dialog.component.scss'],
    standalone: false,
})
export class MfaCertificatesIssuerDialogComponent implements OnInit {
    issuer: string = '';

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        private dialogRef: MatDialogRef<MfaCertificatesIssuerDialogComponent>
    ) { }

    ngOnInit(): void {
        this.issuer = this.data.data;
    }

    closeDialog(certificateType?: string) {
        this.dialogRef.close(certificateType);
    }
}
