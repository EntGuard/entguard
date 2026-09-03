import { Component, EventEmitter, Inject, OnDestroy, OnInit, ViewChild } from '@angular/core';
import { AbstractControl, FormBuilder, FormControl, FormGroup, ValidatorFn, Validators } from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { LDAPService } from '../../ldap.service';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { LdapTemplateModel } from '../../models/ldap-template-model';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { TranslateService } from '@ngx-translate/core';
import { MatStepper } from '@angular/material/stepper';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { LdapInterfaceModel } from '../../models/ldap-template-interface-model';
import { Patterns } from '../../../../shared/validator-patterns';
import { Subject } from 'rxjs';
import { UsersDataService } from '../../../users/users-data.service';
import { AddressPoolModel } from '../../../../shared/models/address-pool.model';

@Component({
    selector: 'app-ldaptemplate-add-dialog',
    templateUrl: './ldaptemplate-add-dialog.component.html',
    styleUrls: ['./ldaptemplate-add-dialog.component.scss'],
    standalone: false,
})
export class LDAPTemplateAddDialogComponent
    extends CommonFormDialogComponent
    implements OnInit, OnDestroy {

    protected componentDestroyed: Subject<void> = new Subject<void>();
    @ViewChild('stepper') stepper: MatStepper;

    formTemplate: FormGroup;
    template: LdapTemplateModel;
    templates: LdapTemplateModel[];
    mfaTypes: Array<string>;
    mfaTypesError: any;
    loading = true;

    formInterface: FormGroup;
    iface: LdapInterfaceModel;
    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    onAddTemplate = new EventEmitter();
    dnsFormControl = new FormControl();
    showPKChars = false;
    addressesControl = new FormControl<AddressPoolModel[]>([]);
    templateCreated = false;
    addressPools: AddressPoolModel[] = [];
    loadingAddressPools = true;
    addressPoolsError: any;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: LDAPService,
        private usersDataService: UsersDataService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService

    ) {
        super(dialogRef, appUtilsService, translateService);
        this.template = data['template'] || new LdapTemplateModel({});
        this.iface = data['iface'] || new LdapInterfaceModel({});
        this.addressesControl.setValidators(this.minArraySelectionValidator(1));
        this.addressesControl.setValue(this.iface.addresses || []);
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

        this.usersDataService
            .getAddressPools()
            .pipe(
                finalize(() => (this.loadingAddressPools = false)),
                takeUntil(this.componentDestroyed))
            .subscribe(
                (res: AddressPoolModel[]) => {
                    this.addressPools = res;
                },
                (error) => (this.addressPoolsError = error)
            );

        this.formTemplate = this.fb.group({
            name: [this.template.name, Validators.required],
            filter: [this.template.filter],
            isAdmin: [this.template.isAdmin],
            mfa_type: [this.template.mfaType],
        });
        this.formInterface = this.fb.group({
            name: [this.iface.name || 'eg0', Validators.required],
            listenPort: [
                this.iface.listenPort,
                Validators.pattern(Patterns.portNumber),
            ],
            mtu: [
                this.iface.mtu,
                [
                    Validators.min(576),
                    Validators.max(65535),
                    Validators.pattern(Patterns.wholeNumber),
                ],
            ],
            addresses: this.addressesControl,
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    onSubmitTemplate() {
        this.error = null;
        const dataObj = {
            name: this.formTemplate.get('name')?.value,
            filter: this.formTemplate.get('filter')?.value,
            is_admin: !!this.formTemplate.get('isAdmin')?.value,
            mfa_type: this.formTemplate.get('mfa_type')?.value || '',
            iface_name: this.formInterface.get('name')?.value,
            address_pools: this.addresses,
            listen_port: this.formInterface.get('listenPort')?.value,
            dns: this.iface.dns,
            mtu: this.formInterface.get('mtu')?.value,
        };

        this.savingProgress = true;

        this.dataService.addTemplate(dataObj).pipe(takeUntil(this.componentDestroyed))
            .subscribe(() => {
                this.dataService
                    .getTemplates()
                    .subscribe(
                        (templates: LdapTemplateModel[]) => {
                            this.templateCreated = true;
                            this.templates = templates;
                            this.onAddTemplate.emit(this.templates);
                            this.savingProgress = false;
                            this.stepper.next();
                        },
                        (err) => {
                            this.templateCreated = true;
                            this.savingProgress = false;
                            this.stepper.next();
                        }
                    );
            },
                (err) => {
                    this.savingProgress = false;
                    this.error = err;
                }
            )
    }

    close() {
        this.dialogRef.close();
    }

    onDone() {
        this.dialogRef.close({ configureTemplateId: undefined });
    }

    onConfigureTemplate() {
        this.dialogRef.close({
            configureTemplateId: this.getTemplateId(this.formTemplate.get('name')?.value),
        });
    }

    onDNSRemove(add: string) {
        const index = this.iface.dns.indexOf(add);

        if (index >= 0) {
            this.iface.dns.splice(index, 1);
        }
    }

    onDnsAdd(event: any) {
        const input = event.input;
        const value = event.value;

        if ((value || '').trim()) {
            this.addUniqueDns(value.trim());
        }

        if (input) {
            input.value = '';
        }
        this.dnsFormControl.setValue(null);
    }

    private addUniqueDns(value: string) {
        if (value && this.iface.dns.indexOf(value) === -1) {
            this.iface.dns.push(value);
        }
    }

    onDnsBlur() {
        this.addUniqueDns(this.dnsFormControl.value);
        this.dnsFormControl.setValue(null);
    }

    getTemplateId(name) {
        const template = this.templates.find((s) => s.name === name);

        if (template) {
            return template.id;
        }

        return null;
    }

    private minArraySelectionValidator(min: number): ValidatorFn {
        return (control: AbstractControl) => {
            const value = control.value as AddressPoolModel[] | null;
            const length = Array.isArray(value) ? value.length : 0;
            return length >= min
                ? null
                : {
                    minArrayLength: {
                        requiredLength: min,
                        actualLength: length,
                    },
                };
        };
    }

    get addresses(): AddressPoolModel[] {
        return this.addressesControl.value || [];
    }
}
