import {
    Component,
    EventEmitter,
    Inject,
    OnDestroy,
    OnInit,
    ViewChild,
} from '@angular/core';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { ServerModel } from '../../../../shared/models/server-model';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { ServersDataService } from '../../servers-data.service';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { switchMap, takeUntil } from 'rxjs/operators';
import { MatStepper } from '@angular/material/stepper';
import { VpnWireguardConfigModel } from '../../../../shared/models/vpn-wireguard-config-model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { Patterns } from 'src/app/shared/validator-patterns';
import { ValidatorsService } from 'src/app/core/validators.service';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-server-dialog',
    templateUrl: './add-server-dialog.component.html',
    styleUrls: ['./add-server-dialog.component.scss'],
    standalone: false,
})
export class AddServerDialogComponent
    extends CommonFormDialogComponent
    implements OnInit, OnDestroy {
    @ViewChild('stepper') stepper: MatStepper;

    createForm: FormGroup;
    server: ServerModel = new ServerModel();
    servers: ServerModel[] = [];
    config: VpnWireguardConfigModel = new VpnWireguardConfigModel();
    savingServer = false;
    configForm: FormGroup;

    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    addressFormControl = new FormControl(
        null,
        this.validatorsService.getIpv4v6CidrValidator()
    );
    onAddServer = new EventEmitter();

    showPKChars = false;

    serverSaved = false;

    specificError = '';

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
    }

    ngOnInit(): void {
        this.createForm = this.fb.group({
            name: [this.server.name, Validators.required],
            endpoint: [
                this.server.endpoint,
                [Validators.required, Validators.pattern(Patterns.IPv4noCIDR)],
            ],
            healthcheckAddress: [
                this.server.healthcheckAddress,
                [Validators.pattern(Patterns.IPv4noCIDR)],
            ],
            description: [this.server.description],
        });

        this.configForm = this.fb.group({
            name: [this.config.interface.name, Validators.required],
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

        this.autogenerateKeys();
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    protected printSpecificError(err: any) {
        if (err && err['error'] && err['error']['message']) {
            this.specificError = err['error']['message'];
        } else {
            this.specificError = err['message'] || err;
            if (!this.specificError) {
                this.translateService.stream('shared.unknownError').pipe(takeUntil(this.componentDestroyed))
                    .subscribe((translatedValue) => {
                        this.specificError = translatedValue;
                    });
            }
        }
    }

    close() {
        this.dialogRef.close();
    }

    autogenerateKeys() {
        if (
            (
                this.configForm.get('privateKey').value +
                this.configForm.get('publicKey').value
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

        this.configForm
            .get('privateKey')
            .setValue(this.appUtilsService.keyToBase64(privkey));
        this.configForm
            .get('publicKey')
            .setValue(this.appUtilsService.keyToBase64(pubkey));
    }

    onAddressRemove(address) {
        const index = this.config.interface.addresses.indexOf(address);

        if (index >= 0) {
            this.config.interface.addresses.splice(index, 1);
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
            this.config.interface.addresses.indexOf(address) === -1 &&
            this.addressFormControl.valid
        ) {
            this.config.interface.addresses.push(address);
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

    submitServer() {
        const dataObj = {
            name: this.createForm.get('name').value,
            endpoint: this.createForm.get('endpoint').value,
            healthcheck_address: this.createForm.get('healthcheckAddress').value,
            description: this.createForm.get('description').value,
            vpn_config: {
                name: this.configForm.get('name').value,
                private_key: this.configForm.get('privateKey').value,
                public_key: this.configForm.get('publicKey').value,
                addresses: this.config.interface.addresses,
                listen_port: this.configForm.get('listenPort').value,
                dns: this.config.interface.dns,
                mtu: this.configForm.get('mtu').value,
                persistent_keepalive: this.configForm.get('persistentKeepalive')
                    .value,
            }
        };

        this.savingServer = true;
        this.dataService
            .postServer(dataObj)
            .pipe(takeUntil(this.componentDestroyed))
            .subscribe(
                () => {
                    this.dataService
                        .getServersList()
                        .pipe(takeUntil(this.componentDestroyed))
                        .subscribe(
                            (res) => {
                                this.servers = res;
                                this.onAddServer.emit(this.servers);
                                this.savingServer = false;
                                this.serverSaved = true;
                                setTimeout(() => this.stepper.next());
                            },
                            (err) => {
                                this.savingServer = false;
                                this.serverSaved = true;
                                this.stepper.next();
                            }
                        );
                },
                (err) => {
                    this.savingServer = false;
                    this.serverSaved = false;
                    this.printSpecificError(err);
                }
            );
    }

    onFinish() {
        this.dialogRef.close({ configureServerId: undefined });
    }

    onConfigureServer() {
        this.dialogRef.close({
            configureServerId: this.getServerId(this.createForm.get('name').value),
        });
    }

    getServerId(name) {
        const server = this.servers.find((s) => s.name === name);

        if (server) {
            return server.id;
        }

        return null;
    }
}
