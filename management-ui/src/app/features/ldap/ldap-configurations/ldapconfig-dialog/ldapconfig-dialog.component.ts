import { Component, Inject, OnInit } from '@angular/core';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { LDAPService } from '../../ldap.service';
import { ValidatorsService } from '../../../../core/validators.service';
import { finalize, takeUntil } from 'rxjs/operators';
import { LdapConfigModel } from '../../models/ldap-config-model';
import { LdapTemplateModel } from '../../models/ldap-template-model';
import { Patterns } from 'src/app/shared/validator-patterns';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-ldapconfig-dialog',
    templateUrl: './ldapconfig-dialog.component.html',
    styleUrls: ['./ldapconfig-dialog.component.scss'],
    standalone: false,
})
export class LDAPConfigDialogComponent
    extends CommonFormDialogComponent
    implements OnInit {
    form: FormGroup;
    config: LdapConfigModel;
    loadingTemplates = true;
    templates: LdapTemplateModel[];
    editing = false;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private myValidators: ValidatorsService,
        private dataService: LDAPService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService
    ) {
        super(dialogRef, appUtilsService, translateService);

        this.config = data['config'];
        this.editing = !!data['config'];
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            priority: [
                this.config ? this.config.priority : '',
                [Validators.required, Validators.pattern(Patterns.wholeNumber)],
            ],
            host: [this.config ? this.config.host : '', [Validators.required]],
            port: [
                this.config ? this.config.port : '',
                [Validators.required, Validators.pattern(Patterns.portNumber)],
            ],
            baseDn: [
                this.config ? this.config.baseDn : '',
                [Validators.required],
            ],
            bindDn: [
                this.config ? this.config.bindDn : '',
                [Validators.required],
            ],
            bindPw: ['', this.editing ? [] : [Validators.required]],
            userListFilter: [
                this.config ? this.config.userListFilter : '',
                [Validators.required],
            ],
            usernameAttr: [
                this.config ? this.config.usernameAttr : '',
                [Validators.required],
            ],
            uidAttr: [
                this.config ? this.config.uidAttr : '',
                [Validators.required],
            ],
            template: [
                this.config ? this.config.template : '', [],
            ],
            useTLS: [
                this.config ? this.config.useTLS : false, []
            ],
            fqdn: [
                this.config ? this.config.fqdn : '', []]
            ,
            caCert: [
                this.config ? this.config.caCert : '', []
            ],
        });
        this.form.addControl(
            'template',
            new FormControl({ value: '', disabled: false }, [
                // TODO
                this.myValidators.getGroupRequirementValidator(this.form, [
                    'fqdn',
                    'caCert',
                ]),
            ])
        );
        this.form.addControl(
            'fqdn',
            new FormControl({ value: '', disabled: true }, [
                this.myValidators.getGroupRequirementValidator(this.form, [
                    'fqdn',
                    'caCert',
                ]),
            ])
        );
        this.form.addControl(
            'caCert',
            new FormControl({ value: '', disabled: true }, [
                this.myValidators.getGroupRequirementValidator(this.form, [
                    'fqdn',
                    'caCert',
                ]),
            ])
        );
        this.checkTLS();
        this.form.get('useTLS').valueChanges.pipe(takeUntil(this.componentDestroyed)).subscribe((val) => {
            this.checkTLS(val);
        });

        this.dataService
            .getTemplates()
            .pipe(
                finalize(() => (this.loadingTemplates = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((templates) => {
                this.templates = templates;
            });
    }

    checkTLS(val: any = this.form.get('useTLS').value) {
        if (val) {
            this.form.get('fqdn').enable();
            this.form.get('caCert').enable();
        } else {
            this.form.get('fqdn').disable();
            this.form.get('caCert').disable();
            this.form.get('fqdn').setValue('');
            this.form.get('caCert').setValue('');
        }
    }

    forceFqdnCacertValidation() {
        this.form.get('fqdn').updateValueAndValidity();
        this.form.get('caCert').updateValueAndValidity();
    }

    submit() {
        const dataObj = {
            priority: parseInt(this.form.get('priority').value, 10),
            host: this.form.get('host').value,
            port: parseInt(this.form.get('port').value, 10),
            base_dn: this.form.get('baseDn').value,
            bind_dn: this.form.get('bindDn').value,
            user_list_filter: this.form.get('userListFilter').value,
            username_attribute: this.form.get('usernameAttr').value,
            uid_attribute: this.form.get('uidAttr').value,
            use_tls: !!this.form.get('useTLS').value,
            template_id: this.form.get('template').value || null,
        };
        if (this.form.get('bindPw')) {
            dataObj['bind_pw'] = this.form.get('bindPw').value;
        }
        if (dataObj['use_tls']) {
            dataObj['fqdn'] = this.form.get('fqdn').value;
            dataObj['ca_cert'] = this.form.get('caCert').value;
        }

        let action = null;
        if (this.editing) {
            action = this.dataService.patchConfig(this.config.id, dataObj);
        } else {
            action = this.dataService.addConfig(dataObj);
        }

        this.performSavingAction(action, this.dataService.getConfigs());
    }

    close() {
        this.dialogRef.close();
    }
}
