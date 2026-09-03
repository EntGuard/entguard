import { Component, Inject, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { LDAPService } from '../../ldap.service';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { LdapTemplateModel } from '../../models/ldap-template-model';
import { finalize, takeUntil } from 'rxjs/operators';
import { TranslateService } from '@ngx-translate/core';
import { Subject } from 'rxjs';

@Component({
    selector: 'app-ldaptemplate-edit-dialog',
    templateUrl: './ldaptemplate-edit-dialog.component.html',
    styleUrls: ['./ldaptemplate-edit-dialog.component.scss'],
    standalone: false,
})
export class LDAPTemplateEditDialogComponent
    extends CommonFormDialogComponent
    implements OnInit, OnDestroy {

    protected componentDestroyed: Subject<void> = new Subject<void>();
    form: FormGroup;
    template: LdapTemplateModel;
    showChars = false;
    mfaTypes: Array<string>;
    mfaTypesError: any;
    loading = true;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: LDAPService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService

    ) {
        super(dialogRef, appUtilsService, translateService);
        this.template = data['template'] || new LdapTemplateModel({});
    }

    ngOnInit(): void {
        this.dataService
            .getMfaTypesList()
            .pipe(
                finalize(() => (this.loading = false)),
                takeUntil(this.componentDestroyed))
            .subscribe(
                (res: Array<string>) => (this.mfaTypes = res),
                (error) => (this.mfaTypesError = error)
            );

        this.form = this.fb.group({
            name: [this.template.name, Validators.required],
            filter: [this.template.filter],
            isAdmin: [this.template.isAdmin],
            mfa_type: [this.template.mfaType],
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    submit() {
        const dataObj = {
            name: this.form.get('name').value,
            filter: this.form.get('filter').value,
            is_admin: !!this.form.get('isAdmin').value,
            mfa_type: this.form.get('mfa_type').value || '',
            iface_name: this.template.interfaceName,
            address_pools: this.template.addressPools,
            listen_port: this.template.listenPort,
            dns: this.template.dns,
            mtu: this.template.mtu,
        };

        let action = this.dataService.patchTemplate(
            this.template.id,
            dataObj
        );

        this.performSavingAction(action, this.dataService.getTemplates());
    }

    close() {
        this.dialogRef.close();
    }
}
