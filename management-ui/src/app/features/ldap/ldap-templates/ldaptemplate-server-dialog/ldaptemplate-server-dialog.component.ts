import { Component, Inject, OnInit } from '@angular/core';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { LDAPService } from '../../ldap.service';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { finalize, takeUntil } from 'rxjs/operators';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { ServerModel } from '../../../../shared/models/server-model';
import { ServersDataService } from '../../../servers/servers-data.service';
import { ValidatorsService } from 'src/app/core/validators.service';
import { LdapTemplateModel } from '../../models/ldap-template-model';
import { LdapServerModel } from '../../models/ldap-template-server-model';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-ldaptemplate-server-dialog',
    templateUrl: './ldaptemplate-server-dialog.component.html',
    styleUrls: ['./ldaptemplate-server-dialog.component.scss'],
    standalone: false,
})
export class LDAPTemplateServerDialogComponent
    extends CommonFormDialogComponent
    implements OnInit {
    form: FormGroup;
    template: LdapTemplateModel;
    server: LdapServerModel;
    oldServerId: number;
    loadingServers = true;
    servers: ServerModel[];
    allowedAddressFC: FormControl = new FormControl(null, [
        this.validatorsService.getIpv4v6CidrValidator(),
    ]);

    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    addresses: string[];
    editing = false;
    error: any;

    private isAddressDirty = false;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: LDAPService,
        private serversDataService: ServersDataService,
        protected appUtilsService: AppUtilsService,
        private validatorsService: ValidatorsService,
        protected translateService: TranslateService

    ) {
        super(dialogRef, appUtilsService, translateService);
        this.template = data['template'] || new LdapTemplateModel({});
        this.server =
            data['server'] ||
            new LdapServerModel({ allowed_ips: [] });
        this.oldServerId = this.server.id;
        this.addresses = Object.assign([], this.server.allowedIps);
        this.editing = !!data['server'];
        this.isAddressDirty = this.editing;
    }

    ngOnInit(): void {
        this.serversDataService
            .getServersList()
            .pipe(
                finalize(() => (this.loadingServers = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (servers: ServerModel[]) => {
                    this.servers = servers;
                },
                (err) => (this.error = err)
            );

        this.form = this.fb.group({
            server: [this.server ? this.server.name : '', Validators.required],
            usePresharedKey: [this.server ? this.server.usePresharedKey : false],
        });

        this.serverChangeSubscribe();
    }

    submit() {
        const dataObj = {
            server_id: this.getServerId(this.form.get('server').value),
            client_side_allowed_ips: this.addresses,
            use_preshared_key: this.form.get('usePresharedKey').value,
        };

        let action = null;
        if (this.editing) {
            action = this.dataService.patchTemplateServer(
                this.template.id,
                this.oldServerId,
                dataObj
            );
        } else {
            action = this.dataService.addTemplateServer(
                this.template.id,
                dataObj
            );
        }

        this.performSavingAction(
            action,
            this.dataService.getTemplateServers(this.template.id)
        );
    }

    close() {
        this.dialogRef.close();
    }

    onAllowedAddressRemove(address: string) {
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
        this.allowedAddressFC.setValue('');
    }

    onAddressesPaste(event: ClipboardEvent): void {
        event.preventDefault();
        event.clipboardData
            .getData('Text')
            .split(/;|,|\s/)
            .forEach((address: string) => {
                if ((address || '').trim()) {
                    this.addUniqueAddress(address.trim());
                }
            });
        this.allowedAddressFC.setValue('');
    }

    addUniqueAddress(address: string): void {
        if (
            address &&
            this.addresses.indexOf(address) === -1 &&
            this.allowedAddressFC.valid
        ) {
            this.addresses.push(address);
            this.isAddressDirty = true;
        } else {
            this.allowedAddressFC.setErrors({ incorrect: true });
        }
    }

    onAddressBlur() {
        this.addUniqueAddress(this.allowedAddressFC.value);
        this.allowedAddressFC.setValue('');
    }

    getServerId(name) {
        const server = this.servers.find((s) => s.name === name);

        if (server) {
            return server.id;
        }

        return null;
    }

    private serverChangeSubscribe(): void {
        this.form.get('server').valueChanges
            .pipe(takeUntil(this.componentDestroyed))
            .subscribe(value => {
                const server = this.servers.find(({ name }) => name === value);

                this.isAddressDirty = false;
                this.addresses = server.healthcheckAddress ? [server.healthcheckAddress] : [];
            });
    }
}
