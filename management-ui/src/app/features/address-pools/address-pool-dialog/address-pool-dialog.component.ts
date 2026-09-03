import { Component, Inject, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators, ValidatorFn, AbstractControl } from '@angular/forms';
import { MatDialogRef, MAT_DIALOG_DATA } from '@angular/material/dialog';
import { UsersDataService } from '../../users/users-data.service';
import { AppUtilsService } from '../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { TranslateService } from '@ngx-translate/core';
import { Subject } from 'rxjs';
import { AddressPoolModel } from '../../../shared/models/address-pool.model';
import { takeUntil } from 'rxjs/operators';
import { ValidatorsService } from '../../../core/validators.service';
import { Patterns } from '../../../shared/validator-patterns';

@Component({
    selector: 'app-address-pool-dialog',
    templateUrl: './address-pool-dialog.component.html',
    styleUrls: ['./address-pool-dialog.component.scss'],
    standalone: false,
})
export class AddressPoolDialogComponent extends CommonFormDialogComponent implements OnInit, OnDestroy {
    protected componentDestroyed: Subject<void> = new Subject<void>();
    form: FormGroup;
    pool: AddressPoolModel;
    editing = false;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: UsersDataService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService,
        private validatorsService: ValidatorsService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.pool = data['pool'] || new AddressPoolModel({});
        if (data['pool']) {
            this.editing = true;
        }
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            name: [this.pool.name, Validators.required],
            addressStart: [
                this.pool.addressStart,
                [Validators.required, this.ipAddressValidator()]
            ],
            addressEnd: [
                this.pool.addressEnd,
                [Validators.required, this.ipAddressValidator()]
            ],
            netmask: [
                this.editing ? this.pool.netmask : null,
                [Validators.required, this.netmaskValidator()]
            ],
            description: [this.pool.description]
        });

        // Update netmask validation when address changes
        this.form
            .get('addressStart')
            ?.valueChanges.pipe(takeUntil(this.componentDestroyed))
            .subscribe(() => {
                this.form.get('netmask')?.updateValueAndValidity();
            });
        this.form
            .get('addressEnd')
            ?.valueChanges.pipe(takeUntil(this.componentDestroyed))
            .subscribe(() => {
                this.form.get('netmask')?.updateValueAndValidity();
            });
    }

    private getAddressFamily(value: unknown): 4 | 6 | null {
        if (typeof value !== 'string') {
            return null;
        }
        const trimmed = value.trim();
        if (!trimmed) {
            return null;
        }
        if (Patterns.IPv4noCIDR.test(trimmed)) {
            return 4;
        }
        if (Patterns.IPv6noCIDR.test(trimmed)) {
            return 6;
        }
        return null;
    }

    private ipAddressValidator(): ValidatorFn {
        return (control: AbstractControl): { [key: string]: any } | null => {
            if (!control.value) {
                return null;
            }
            // Use patterns from the validator-patterns file to validate IP addresses without CIDR
            if (Patterns.IPv4noCIDR.test(control.value)) {
                return null; // Valid IPv4
            }
            if (Patterns.IPv6noCIDR.test(control.value)) {
                return null; // Valid IPv6 without CIDR
            }
            return { 'ipAddress': true }; // Invalid
        };
    }

    private netmaskValidator(): ValidatorFn {
        return (control: AbstractControl): { [key: string]: any } | null => {
            if (!control.value && control.value !== 0) {
                return null;
            }
            const netmask = parseInt(control.value, 10);
            if (isNaN(netmask) || netmask < 0) {
                return { 'pattern': true };
            }

            // Determine max CIDR based on address type.
            // Use both start/end to avoid mis-detecting when only one is filled/valid.
            const addressStart = this.form?.get('addressStart')?.value;
            const addressEnd = this.form?.get('addressEnd')?.value;
            const startFamily = this.getAddressFamily(addressStart);
            const endFamily = this.getAddressFamily(addressEnd);
            const maxCidr = startFamily === 6 || endFamily === 6 ? 128 : 32;

            if (netmask > maxCidr) {
                return { 'pattern': true };
            }
            return null;
        };
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
        this.componentDestroyed.complete();
    }

    submit() {
        const dataObj = {
            name: this.form.get('name')!.value,
            start_addr: this.form.get('addressStart')!.value,
            end_addr: this.form.get('addressEnd')!.value,
            net_mask: parseInt(this.form.get('netmask')!.value, 10),
            description: this.form.get('description')!.value,
        };
        let action;

        if (this.editing) {
            action = this.dataService.patchAddressPool(this.pool.id, dataObj);
        } else {
            action = this.dataService.postAddressPool(dataObj);
        }

        this.performSavingAction(action);
    }

    close() {
        this.dialogRef.close();
    }
}
