import { Component, Inject, OnInit, OnDestroy } from '@angular/core';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { Patterns } from '../../../../shared/validator-patterns';
import { ValidatorsService } from '../../../../core/validators.service';
import { LdapTemplateModel } from '../../models/ldap-template-model';
import { LdapInterfaceModel } from '../../models/ldap-template-interface-model';
import { LDAPService } from '../../ldap.service';
import { TranslateService } from '@ngx-translate/core';
import { UsersDataService } from '../../../users/users-data.service';
import { AddressPoolModel } from '../../../../shared/models/address-pool.model';
import { Subject, of } from 'rxjs';
import { finalize, takeUntil } from 'rxjs/operators';

@Component({
    selector: 'app-user-config-dialog',
    templateUrl: './ldaptemplate-interface-dialog.component.html',
    styleUrls: ['./ldaptemplate-interface-dialog.component.scss'],
    standalone: false,
})
export class LDAPTemplateInterfaceDialogComponent
    extends CommonFormDialogComponent
    implements OnInit, OnDestroy {
    protected componentDestroyed: Subject<void> = new Subject<void>();
    form: FormGroup;
    template: LdapTemplateModel;
    iface: LdapInterfaceModel;
    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];

    addressesControl = new FormControl<AddressPoolModel[]>([], [Validators.required]);
    dnsFormControl = new FormControl();

    addresses: AddressPoolModel[];
    editing = false;
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
        private validatorsService: ValidatorsService,
        protected translateService: TranslateService

    ) {
        super(dialogRef, appUtilsService, translateService);
        this.template = data['template'];
        this.iface = data['iface'] || new LdapInterfaceModel({});
        this.addresses = this.iface.addresses || [];
        this.editing = !!data['iface'];
    }

    ngOnInit(): void {
        this.usersDataService
            .getAddressPools()
            .pipe(
                finalize(() => (this.loadingAddressPools = false)),
                takeUntil(this.componentDestroyed))
            .subscribe(
                (res: AddressPoolModel[]) => {
                    this.addressPools = res;
                    this.addresses = this.addresses.map(existingAddr => {
                        const matchingPool = this.addressPools.find(pool =>
                            pool.addressRange === existingAddr.addressRange
                        );
                        return matchingPool || existingAddr;
                    });
                    this.addressesControl.setValue(this.addresses);
                },
                (error) => (this.addressPoolsError = error)
            );

        // Ensure DNS array is initialized
        if (!this.iface.dns) {
            this.iface.dns = [];
        }

        this.form = this.fb.group({
            name: [this.iface.name || (!this.editing ? 'eg0' : ''), Validators.required],
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
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    onSubmit() {
        const dataObj = {
            iface_name: this.form.get('name')?.value,
            address_pools: this.addressesControl.value,
            listen_port: this.form.get('listenPort')?.value,
            dns: this.iface.dns,
            mtu: this.form.get('mtu')?.value,
            name: this.template.name,
            filter: this.template.filter,
            is_admin: this.template.isAdmin,
            mfa_type: this.template.mfaType,
        };

        const updatedInterface = new LdapInterfaceModel({
            iface_name: dataObj.iface_name,
            iface_addrs: dataObj.address_pools,
            listen_port: dataObj.listen_port,
            dns: dataObj.dns,
            mtu: dataObj.mtu
        });

        this.performSavingAction(
            this.dataService.patchTemplate(this.template.id, dataObj),
            of(updatedInterface)
        );
    }

    onClose() {
        this.dialogRef.close();
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
}
