import { Component, Inject, OnInit } from '@angular/core';
import {
    FormBuilder,
    FormControl,
    FormGroup,
} from '@angular/forms';
import { ServerModel } from '../../../../shared/models/server-model';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { ServersDataService } from '../../servers-data.service';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { ServerAdjacencyModel } from '../../../../shared/models/server-adjacency-model';
import { Observable } from 'rxjs';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { ValidatorsService } from '../../../../core/validators.service';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-server-adjacency-dialog',
    templateUrl: './server-adjacency-dialog.component.html',
    styleUrls: ['./server-adjacency-dialog.component.scss'],
    standalone: false,
})
export class ServerAdjacencyDialogComponent
    extends CommonFormDialogComponent
    implements OnInit {
    form: FormGroup;
    server: ServerModel;
    adjacency: ServerAdjacencyModel;
    editing = false;

    deviceId: number;
    addresses: string[];
    otherSideAddresses: string[];
    allowedAddressFC: FormControl;
    otherSideAllowedAddressFC: FormControl;

    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: ServersDataService,
        protected appUtilsService: AppUtilsService,
        private validatorsService: ValidatorsService,
        protected translateService: TranslateService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.server = data['server'] || new ServerModel({});
        this.adjacency =
            data['adjacency'] ||
            new ServerAdjacencyModel({
                config: {
                    allowed_ips: [],
                    other_side_allowed_ips: this.server.healthcheckAddress ? [this.server.healthcheckAddress] : [],
                },
            });
        this.addresses = Object.assign([], this.adjacency.allowedIps);
        this.otherSideAddresses = Object.assign(
            [],
            this.adjacency.otherSideAllowedIps
        );
        if (data['adjacency'] && this.adjacency.device) {
            this.deviceId = this.adjacency.device.id;
        }
        this.editing = !!data['adjacency'];

        this.allowedAddressFC = new FormControl(
            null,
            this.validatorsService.getIpv4v6CidrValidator()
        );
        this.otherSideAllowedAddressFC = new FormControl(
            null,
            this.validatorsService.getIpv4v6CidrValidator()
        );
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            allowedIps: [this.adjacency ? this.addresses : ''],
            otherSideAllowedIps: [
                this.adjacency ? this.otherSideAddresses : '',
            ],
            presharedKey: [this.adjacency ? this.adjacency.presharedKey : ''],
        });
    }

    submit() {
        const dataObj = {
            device_id: this.deviceId,
            server_id: this.server.id,
            config: {
                allowed_ips: this.addresses,
                other_side_allowed_ips: this.otherSideAddresses,
                preshared_key: this.form.get('presharedKey')!.value,
                server_side: true,
            },
        };

        let saveAction: Observable<void> | null = null;
        if (this.editing) {
            saveAction = this.dataService.patchServerAdjacency(
                this.adjacency.id,
                dataObj
            );
        } else {
            saveAction = this.dataService.postAdjacency(dataObj);
        }

        this.performSavingAction(
            saveAction,
            this.dataService.getServerAdjacencies(this.server.id)
        );
    }

    close() {
        this.dialogRef.close();
    }

    onAllowedAddressRemove(
        address: string,
        addressesArray: 'addresses' | 'otherSideAddresses'
    ) {
        const index = this[addressesArray].indexOf(address);
        if (index >= 0) {
            this[addressesArray].splice(index, 1);
        }
    }

    onMatChipInputTokenEnd(
        addressesArray: 'addresses' | 'otherSideAddresses',
        addressFC: 'allowedAddressFC' | 'otherSideAllowedAddressFC'
    ) {
        if ((this[addressFC].value || '').trim()) {
            this.addUniqueAddress(
                this[addressFC].value,
                addressesArray,
                addressFC
            );
        }

        this[addressFC].setValue('');
    }

    onAddressesPaste(
        event: ClipboardEvent,
        addressesArray: 'addresses' | 'otherSideAddresses',
        addressFC: 'allowedAddressFC' | 'otherSideAllowedAddressFC'
    ): void {
        event.preventDefault();
        const clipboardData = event.clipboardData;
        if (clipboardData) {
            clipboardData
                .getData('Text')
                .split(/;|,|\s/)
                .forEach((address: string) => {
                    if ((address || '').trim()) {
                        this.addUniqueAddress(
                            address.trim(),
                            addressesArray,
                            addressFC
                        );
                    }
                });
        }

        this[addressFC].setValue('');
    }

    private addUniqueAddress(
        address: string,
        addressesArray: 'addresses' | 'otherSideAddresses',
        addressFC: 'allowedAddressFC' | 'otherSideAllowedAddressFC'
    ): void {
        if (
            address &&
            this[addressesArray].indexOf(address) === -1 &&
            this[addressFC].valid
        ) {
            this[addressesArray].push(address);
        } else {
            this[addressFC].setErrors({ incorrect: true });
        }
    }

    onAddressBlur(
        addressesArray: 'addresses' | 'otherSideAddresses',
        addressFC: 'allowedAddressFC' | 'otherSideAllowedAddressFC'
    ) {
        this.addUniqueAddress(this[addressFC].value, addressesArray, addressFC);
        this[addressFC].setValue('');
    }
}
