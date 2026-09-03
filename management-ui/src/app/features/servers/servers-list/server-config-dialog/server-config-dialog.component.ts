import { Component, Inject, OnDestroy, OnInit } from '@angular/core';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { ServerModel } from '../../../../shared/models/server-model';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { ServersDataService } from '../../servers-data.service';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { VpnWireguardConfigModel } from '../../../../shared/models/vpn-wireguard-config-model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { ValidatorsService } from 'src/app/core/validators.service';
import { Patterns } from 'src/app/shared/validator-patterns';
import { TranslateService } from '@ngx-translate/core';
import { switchMap, takeUntil } from 'rxjs/operators';

@Component({
    selector: 'app-server-config-dialog',
    templateUrl: './server-config-dialog.component.html',
    styleUrls: ['./server-config-dialog.component.scss'],
    standalone: false,
})
export class ServerConfigDialogComponent
    extends CommonFormDialogComponent
    implements OnInit, OnDestroy {
    form: FormGroup;
    server: ServerModel;
    config: VpnWireguardConfigModel;
    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    addressFormControl = new FormControl(
        null,
        [
            Validators.required,
            this.validatorsService.getIpv4v6CidrValidator()
        ]
    );
    showPKChars = false;
    addresses = [];

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
        this.server = data['server'];
        this.config = data['config'];
        this.addresses = Object.assign([], this.config.interface.addresses);
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            name: [this.config.interface.name || 'eg0', Validators.required],
            privateKey: [this.config.interface.privateKey, Validators.required],
            publicKey: [this.config.interface.publicKey, Validators.required],
            listenPort: [
                this.config.interface.listenPort,
                [Validators.required, Validators.pattern(Patterns.portNumber)],
            ],
            mtu: [
                this.config.interface.mtu,
                [
                    Validators.min(576),
                    Validators.max(65535),
                    Validators.pattern(Patterns.wholeNumber),
                ],
            ],
            persistentKeepalive: [
                this.config.interface.persistentKeepalive,
                [
                    Validators.min(1),
                    Validators.max(65535),
                    Validators.pattern(Patterns.wholeNumber),
                ],
            ],
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    submit() {
        const dataObj = {
            name: this.form.get('name').value,
            private_key: this.form.get('privateKey').value,
            public_key: this.form.get('publicKey').value,
            addresses: this.addresses,
            listen_port: this.form.get('listenPort').value,
            dns: this.config.interface.dns,
            mtu: this.form.get('mtu').value,
            persistent_keepalive: this.form.get('persistentKeepalive')
                .value,
        };

        let action = this.dataService.patchServerConfiguration(
            this.server.id,
            dataObj
        );

        this.performSavingAction(
            action,
            this.dataService.getServerVpnConfig(this.server.id)
        );
    }

    close() {
        this.dialogRef.close();
    }

    autogenerateKeys() {
        if (
            (
                this.form.get('privateKey').value +
                this.form.get('publicKey').value
            ).length > 0
        ) {
            this.translateService.stream('shared.confirmOverwrite')
                .pipe(
                    switchMap((translatedText) =>
                        this.appUtilsService.showConfirmation(translatedText)
                    ),
                    takeUntil(this.componentDestroyed)
                )
                .subscribe((result) => result && this.generateAndFillKeys());
        } else {
            this.generateAndFillKeys();
        }
    }

    private generateAndFillKeys() {
        const privkey = this.appUtilsService.generatePrivateKey();
        const pubkey = this.appUtilsService.generatePublicKey(privkey);

        this.form
            .get('privateKey')
            .setValue(this.appUtilsService.keyToBase64(privkey));
        this.form
            .get('publicKey')
            .setValue(this.appUtilsService.keyToBase64(pubkey));
    }

    onAddressRemove(address: any) {
        const index = this.addresses.indexOf(address);

        if (index >= 0) {
            this.addresses.splice(index, 1);
        }
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

        this.addressFormControl.setValue(null);
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
        this.addressFormControl.setValue(null);
    }

    private addUniqueAddress(address: string) {
        if (
            address &&
            this.addresses.indexOf(address) === -1 &&
            this.addressFormControl.valid
        ) {
            this.addresses.push(address);
        } else {
            this.addressFormControl.setErrors({ incorrect: true });
        }
    }

    toggleCharsShow() {
        this.showPKChars = !this.showPKChars;
    }

    onBlurAddress() {
        this.addUniqueAddress(this.addressFormControl.value);
        this.addressFormControl.setValue('');
    }
}
