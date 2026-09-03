import { Component, Inject, OnDestroy, OnInit } from '@angular/core';
import {
    AbstractControl,
    FormBuilder,
    FormControl,
    FormGroup,
    ValidatorFn,
    Validators,
} from '@angular/forms';
import { UserModel } from '../../../../../shared/models/user-model';
import { AddressPoolModel } from '../../../../../shared/models/address-pool.model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { UsersDataService } from '../../../../users/users-data.service';
import { AppUtilsService } from '../../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { Patterns } from '../../../../../shared/validator-patterns';
import { TranslateService } from '@ngx-translate/core';
import { finalize, takeUntil } from 'rxjs/operators';
import { DeviceTemplateModel } from '../../../../../shared/models/device-template.model';

@Component({
    selector: 'app-user-config-dialog',
    templateUrl: './user-config-dialog.component.html',
    styleUrls: ['./user-config-dialog.component.scss'],
    standalone: false,
})
export class UserConfigDialogComponent
    extends CommonFormDialogComponent
    implements OnInit, OnDestroy {
    form: FormGroup;
    user: UserModel;
    template: DeviceTemplateModel;
    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    dnsFormControl = new FormControl();
    editing = false;

    addressPools: AddressPoolModel[] = [];
    loadingAddressPools = true;
    addressesControl = new FormControl<AddressPoolModel[]>([], this.minArraySelectionValidator(1));

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: UsersDataService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.user = data['user'];
        this.template = new DeviceTemplateModel(data['template'] || {});
        this.addressesControl.setValue(this.template.addressPools || []);
        this.editing = !!data['template'];
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            name: [this.template.interfaceName || (!this.editing ? 'eg0' : ''), Validators.required],
            listenPort: [
                this.template.listenPort,
                Validators.pattern(Patterns.portNumber),
            ],
            mtu: [
                this.template.mtu,
                [
                    Validators.min(576),
                    Validators.max(65535),
                    Validators.pattern(Patterns.wholeNumber),
                ],
            ],
            addresses: this.addressesControl,
        });

        this.loadAddressPools();
    }

    private loadAddressPools(): void {
        this.loadingAddressPools = true;
        this.dataService.getAddressPools()
            .pipe(
                finalize(() => this.loadingAddressPools = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (pools) => {
                    this.addressPools = pools;
                    const selected = (this.template.addressPools || []).map((pool) =>
                        pools.find((p) => p.id === pool.id) || pool
                    );
                    this.addressesControl.setValue(selected);
                },
                (err) => this.appUtilsService.handleHttpError(err)
            );
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    submit() {
        const dataObj = {
            interface_name: this.form.get('name')?.value,
            address_pools: this.addresses,
            listen_port: this.form.get('listenPort')?.value,
            dns: this.template.dns,
            mtu: this.form.get('mtu')?.value,
        };
        const action = this.editing
            ? this.dataService.updateUserDeviceTemplate(this.user.id, dataObj)
            : this.dataService.createUserDeviceTemplate(this.user.id, dataObj);
        this.performSavingAction(
            action,
            this.dataService.getUserDeviceTemplate(this.user.id)
        );
    }

    close() {
        this.dialogRef.close();
    }

    onDNSRemove(add: string) {
        const index = this.template.dns.indexOf(add);

        if (index >= 0) {
            this.template.dns.splice(index, 1);
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
        if (value && this.template.dns.indexOf(value) === -1) {
            this.template.dns.push(value);
        }
    }

    onDnsBlur() {
        this.addUniqueDns(this.dnsFormControl.value);
        this.dnsFormControl.setValue(null);
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
