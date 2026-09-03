import { Component, Inject, OnInit } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

@Component({
    selector: 'app-ldap-errors',
    templateUrl: './ldap-errors.component.html',
    styleUrls: ['./ldap-errors.component.scss'],
    standalone: false,
})
export class LDAPErrorsComponent implements OnInit {
    errorsStr = '';

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        private dialogRef: MatDialogRef<LDAPErrorsComponent>
    ) { }

    ngOnInit(): void {
        this.errorsStr = this.data.errors.join("\n");
    }

    closeDialog(certificateType?: string) {
        this.dialogRef.close(certificateType);
    }
}
