import { Component, Inject, OnInit, ViewChild } from '@angular/core';
import { FormBuilder, FormControl, FormGroup, Validators } from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { MatChipGrid } from '@angular/material/chips';
import { UsersDataService } from '../../../users-data.service';
import { AppUtilsService } from '../../../../../core/app-utils.service';
import { TranslateService } from '@ngx-translate/core';
import { CommonFormDialogComponent } from '../../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { DeviceConfigurationModel } from '../../../../../shared/models/device-configuration.model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { ValidatorsService } from '../../../../../core/validators.service';
import { switchMap, takeUntil } from 'rxjs/operators';

@Component({
    selector: 'app-device-dialog',
    templateUrl: './device-dialog.component.html',
    styleUrls: ['./device-dialog.component.scss'],
    standalone: false
})
export class DeviceDialogComponent extends CommonFormDialogComponent implements OnInit {
    form: FormGroup;
    device: DeviceConfigurationModel;
    deviceTemplateId: number;
    editing = false;
    loading = false;

    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    addresses: string[] = [];
    addressFC: FormControl = new FormControl(null, this.validatorsService.getIpv4v6CidrValidator());
    showPKChars = false;

    @ViewChild('chipList') chipGrid: MatChipGrid;

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
        this.deviceTemplateId = data.deviceTemplateId;
        this.device = data.device ? new DeviceConfigurationModel(data.device) : new DeviceConfigurationModel();
        this.editing = !!data.device;
        this.addresses = this.device.addresses ? [...this.device.addresses] : [];
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            description: [this.device.description],
            publicKey: [this.device.publicKey, Validators.required],
            privateKey: [this.device.privateKey, Validators.required]
        });

        if (!this.editing) {
            this.loading = true;
            this.dataService.autogenDevice(this.deviceTemplateId)
                .pipe(takeUntil(this.componentDestroyed))
                .subscribe(
                    (autogen) => {
                        this.form.patchValue({
                            description: autogen.description,
                            publicKey: autogen.publicKey,
                            privateKey: autogen.privateKey
                        });
                        this.addresses = autogen.addresses ? [...autogen.addresses] : [];
                        this.loading = false;
                    },
                    (err) => {
                        this.printError(err);
                        this.loading = false;
                    }
                );
        }
    }

    get isSubmitDisabled(): boolean {
        return this.savingProgress || this.form.invalid || this.addresses.length === 0;
    }

    submit() {
        let action;
        const deviceData = {
            description: this.form.get('description').value,
            public_key: this.form.get('publicKey').value,
            private_key: this.form.get('privateKey').value,
            addresses: this.addresses
        };

        if (this.editing) {
            action = this.dataService.updateDevice(this.device.id, deviceData);
        } else {
            action = this.dataService.createDevice(this.deviceTemplateId, deviceData);
        }

        this.performSavingAction(action, this.dataService.getDevices(this.deviceTemplateId));
    }

    autogenerateKeys(): void {
        if ((this.form.get('privateKey').value + this.form.get('publicKey').value).length > 0) {
            this.translateService
                .stream('shared.confirmOverwrite')
                .pipe(
                    switchMap(translatedText =>
                        this.appUtilsService.showConfirmation(translatedText)
                    ),
                    takeUntil(this.componentDestroyed)
                )
                .subscribe(result => result && this.generateAndFillKeys());
        } else {
            this.generateAndFillKeys();
        }
    }

    private generateAndFillKeys(): void {
        const privkey = this.appUtilsService.generatePrivateKey();
        const pubkey = this.appUtilsService.generatePublicKey(privkey);

        this.form.get('privateKey').setValue(this.appUtilsService.keyToBase64(privkey));
        this.form.get('publicKey').setValue(this.appUtilsService.keyToBase64(pubkey));
    }

    close() {
        this.dialogRef.close();
    }

    onAddressRemove(address: string) {
        const index = this.addresses.indexOf(address);
        if (index >= 0) {
            this.addresses.splice(index, 1);
        }
        (this.chipGrid as any)?._markAsTouched?.();
    }

    onAddressAdd(event: any) {
        const input = event.input;
        const value = event.value;

        if ((value || '').trim()) {
            this.addUniqueAddress(value.trim());
        }

        if (input) {
            input.value = '';
        }
        this.addressFC.setValue(null);
    }

    addUniqueAddress(address: string): void {
        if (address && this.addresses.indexOf(address) === -1 && this.addressFC.valid) {
            this.addresses.push(address);
        } else if (!this.addressFC.valid) {
            this.addressFC.setErrors({ incorrect: true });
        }
    }

    onAddressBlur() {
        if (this.addressFC.value) {
            this.addUniqueAddress(this.addressFC.value);
            this.addressFC.setValue(null);
        }
        (this.chipGrid as any)?._markAsTouched?.();
    }

    onAddressesPaste(event: ClipboardEvent): void {
        event.preventDefault();
        event.clipboardData
            .getData('Text')
            .split(/;|,|\s/)
            .forEach((address) => {
                if ((address || '').trim()) {
                    this.addUniqueAddress(address.trim());
                }
            });
        this.addressFC.setValue(null);
    }

    toggleCharsShow(): void {
        this.showPKChars = !this.showPKChars;
    }
}
