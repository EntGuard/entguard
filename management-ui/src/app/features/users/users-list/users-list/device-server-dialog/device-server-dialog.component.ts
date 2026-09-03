import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormControl, FormGroup, Validators } from '@angular/forms';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { UsersDataService } from '../../../users-data.service';
import { ServersDataService } from '../../../../servers/servers-data.service';
import { AppUtilsService } from '../../../../../core/app-utils.service';
import { TranslateService } from '@ngx-translate/core';
import { CommonFormDialogComponent } from '../../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { ServerModel } from '../../../../../shared/models/server-model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { ValidatorsService } from '../../../../../core/validators.service';
import { finalize, takeUntil } from 'rxjs/operators';

import { DeviceConfigurationModel } from '../../../../../shared/models/device-configuration.model';

export class DeviceServerAdjacencyModel {
    id: number;
    server: ServerModel;
    device: DeviceConfigurationModel;
    allowedIps: string[];
    otherSideAllowedIps: string[];
    presharedKey: string;
    persistentKeepalive: string;

    constructor(data: any = {}) {
        this.id = data.id;
        this.server = data.server ? new ServerModel(data.server) : null;
        this.device = data.device ? new DeviceConfigurationModel(data.device) : null;
        // Handle both API response format (top-level camelCase) and config format (nested snake_case)
        this.allowedIps = data.allowedIps || (data.config ? data.config.allowed_ips || [] : []);
        this.otherSideAllowedIps = data.otherSideAllowedIps || (data.config ? data.config.other_side_allowed_ips || [] : []);
        this.presharedKey = data.presharedKey || (data.config ? data.config.preshared_key || '' : '');
        this.persistentKeepalive = data.persistentKeepalive || (data.config ? data.config.persistent_keepalive : '');
    }
}

@Component({
    selector: 'app-device-server-dialog',
    templateUrl: './device-server-dialog.component.html',
    styleUrls: ['./device-server-dialog.component.scss'],
    standalone: false
})
export class DeviceServerDialogComponent extends CommonFormDialogComponent implements OnInit {
    form: FormGroup;
    adjacency: DeviceServerAdjacencyModel;
    deviceId: number;
    userId: number;
    devices: DeviceConfigurationModel[] = [];
    editing = false;
    loadingServers = true;
    servers: ServerModel[] = [];

    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    allowedIps: string[] = [];
    otherSideAllowedIps: string[] = [];

    allowedIpsFC: FormControl = new FormControl(null, this.validatorsService.getIpv4v6CidrValidator());
    otherSideAllowedIpsFC: FormControl = new FormControl(null, this.validatorsService.getIpv4v6CidrValidator());

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: UsersDataService,
        private serversDataService: ServersDataService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService,
        private validatorsService: ValidatorsService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.deviceId = data.deviceId;
        this.userId = data.userId;
        this.devices = data.devices || [];
        this.adjacency = data.adjacency ? new DeviceServerAdjacencyModel(data.adjacency) : new DeviceServerAdjacencyModel({});
        this.editing = !!data.adjacency;
        this.allowedIps = this.adjacency.allowedIps ? [...this.adjacency.allowedIps] : [];
        this.otherSideAllowedIps = this.adjacency.otherSideAllowedIps ? [...this.adjacency.otherSideAllowedIps] : [];

        if (this.editing && this.adjacency.device) {
            this.deviceId = this.adjacency.device.id;
        }
    }

    ngOnInit(): void {
        this.serversDataService.getServersList()
            .pipe(
                finalize(() => this.loadingServers = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (servers) => {
                    this.servers = servers;
                    if (this.servers.length === 1 && !this.editing) {
                        this.form.get('server').setValue(this.servers[0].name);
                    }
                },
                (err) => this.appUtilsService.handleHttpError(err)
            );

        this.form = this.fb.group({
            server: [this.adjacency.server ? this.adjacency.server.name : '', Validators.required],
            device: [this.deviceId || '', Validators.required],
            presharedKey: [this.adjacency.presharedKey || ''],
            persistentKeepalive: [this.adjacency.persistentKeepalive]
        });

        // Listen to server selection changes to pre-populate client-side allowed IPs
        this.form.get('server').valueChanges
            .pipe(takeUntil(this.componentDestroyed))
            .subscribe((serverName) => {
                if (!this.editing && serverName) {
                    this.onServerSelected(serverName);
                }
            });

        // Listen to device selection changes to pre-populate server-side allowed IPs
        this.form.get('device').valueChanges
            .pipe(takeUntil(this.componentDestroyed))
            .subscribe((deviceId) => {
                if (!this.editing && deviceId) {
                    this.onDeviceSelected(deviceId);
                }
            });

        // Auto-select device if there's only one and not editing
        if (this.devices.length === 1 && !this.editing && !this.deviceId) {
            this.form.get('device').setValue(this.devices[0].id);
        }
    }

    private onServerSelected(serverName: string): void {
        const server = this.servers.find(s => s.name === serverName);
        if (server.healthcheckAddress) {
            this.allowedIps = [server.healthcheckAddress];
        }
    }

    private onDeviceSelected(deviceId: number): void {
        const device = this.devices.find(d => d.id === deviceId);
        if (device?.addresses?.length > 0) {
            // Pre-populate server-side allowed IPs with device addresses
            this.otherSideAllowedIps = [...device.addresses];
        }
    }

    submit() {
        const server = this.servers.find(s => s.name === this.form.get('server').value);
        if (!server) return;

        const selectedDeviceId = this.form.get('device').value;

        const adjacencyData = {
            server_id: server.id,
            device_id: selectedDeviceId,
            config: {
                /* Allowed / Other side are switched since server_side false doesnt allow other_side changing
                but from the servers perspective adj allowed/other_side are flipped compared to users adj */
                allowed_ips: this.otherSideAllowedIps,
                other_side_allowed_ips: this.allowedIps,
                preshared_key: this.form.get('presharedKey').value,
                persistent_keepalive: this.form.get('persistentKeepalive').value,
                server_side: true
            }
        };

        let action;
        if (this.editing) {
            action = this.dataService.patchDeviceServerAdjacency(this.adjacency.id, adjacencyData);
        } else {
            action = this.dataService.postDeviceServerAdjacency(adjacencyData);
        }

        this.performSavingAction(action, this.dataService.getUserDeviceServerAdjacencies(this.userId));
    }

    close() {
        this.dialogRef.close();
    }

    onAllowedIpRemove(address: string) {
        const index = this.allowedIps.indexOf(address);
        if (index >= 0) {
            this.allowedIps.splice(index, 1);
        }
    }

    onAllowedIpAdd(event: any) {
        const input = event.input;
        const value = event.value;

        if ((value || '').trim()) {
            this.addUniqueAddress(value.trim(), this.allowedIps, this.allowedIpsFC);
        }

        if (input) {
            input.value = '';
        }
        this.allowedIpsFC.setValue(null);
    }

    onOtherSideAllowedIpRemove(address: string) {
        const index = this.otherSideAllowedIps.indexOf(address);
        if (index >= 0) {
            this.otherSideAllowedIps.splice(index, 1);
        }
    }

    onOtherSideAllowedIpAdd(event: any) {
        const input = event.input;
        const value = event.value;

        if ((value || '').trim()) {
            this.addUniqueAddress(value.trim(), this.otherSideAllowedIps, this.otherSideAllowedIpsFC);
        }

        if (input) {
            input.value = '';
        }
        this.otherSideAllowedIpsFC.setValue(null);
    }

    addUniqueAddress(address: string, list: string[], fc: FormControl): void {
        if (address && list.indexOf(address) === -1 && fc.valid) {
            list.push(address);
        } else if (!fc.valid) {
            fc.setErrors({ incorrect: true });
        }
    }

    onAddressBlur(list: string[], fc: FormControl) {
        if (fc.value) {
            this.addUniqueAddress(fc.value, list, fc);
            fc.setValue(null);
        }
    }

    onAddressesPaste(event: ClipboardEvent, list: string[], fc: FormControl): void {
        event.preventDefault();
        event.clipboardData
            .getData('Text')
            .split(/;|,|\s/)
            .forEach((address) => {
                if ((address || '').trim()) {
                    this.addUniqueAddress(address.trim(), list, fc);
                }
            });
        fc.setValue(null);
    }
}
