import { Component, Inject, OnInit } from '@angular/core';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { UserModel } from '../../../../../shared/models/user-model';
import { AdjacencyTemplateModel } from '../../../../../shared/models/adjacency-template.model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { UsersDataService } from '../../../users-data.service';
import { AppUtilsService } from '../../../../../core/app-utils.service';
import { finalize, takeUntil } from 'rxjs/operators';
import { CommonFormDialogComponent } from '../../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { ServerModel } from '../../../../../shared/models/server-model';
import { ServersDataService } from '../../../../servers/servers-data.service';
import { ValidatorsService } from 'src/app/core/validators.service';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-user-adjacency-dialog',
    templateUrl: './user-adjacency-dialog.component.html',
    styleUrls: ['./user-adjacency-dialog.component.scss'],
    standalone: false,
})
export class UserAdjacencyDialogComponent
    extends CommonFormDialogComponent
    implements OnInit {
    form: FormGroup;
    user: UserModel;
    adjacency: AdjacencyTemplateModel;
    loadingServers = true;
    servers: ServerModel[];
    allowedAddressFC: FormControl = new FormControl(
        null,
        this.validatorsService.getIpv4v6CidrValidator()
    );

    separatorKeysCodes: number[] = [ENTER, COMMA, SEMICOLON, SPACE];
    addresses: string[];
    editing = false;

    private isAddressDirty = false;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: UsersDataService,
        private serversDataService: ServersDataService,
        protected appUtilsService: AppUtilsService,
        private validatorsService: ValidatorsService,
        protected translateService: TranslateService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.user = data['user'] || new UserModel({});
        this.adjacency =
            data['adjacency'] ||
            new AdjacencyTemplateModel({ template_config: { client_side_allowed_ips: [] } });
        this.addresses = Object.assign([], this.adjacency.allowedIps);
        this.editing = !!data['adjacency'];
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
                (servers) => {
                    this.servers = servers;
                    if (this.servers.length === 1 && !this.editing) {
                        this.form.get('server').setValue(this.servers[0].name);
                    }
                },
                (err) => this.appUtilsService.handleHttpError(err)
            );

        this.form = this.fb.group({
            server: [
                this.adjacency && this.adjacency.server
                    ? this.adjacency.server.name
                    : '',
                Validators.required,
            ],
            usePresharedKey: [this.adjacency ? this.adjacency.usePresharedKey : false],
        });

        this.serverChangeSubscribe();
    }

    submit() {
        const dataObj = {
            server_id: this.getServerId(this.form.get('server').value),
            user_id: this.user.id,
            template_config: {
                use_preshared_key: this.form.get('usePresharedKey').value,
                client_side_allowed_ips: this.addresses,
            },
        };

        let action = null;
        if (this.editing) {
            action = this.dataService.updateAdjacencyTemplate(
                this.adjacency.id,
                dataObj
            );
        } else {
            action = this.dataService.createAdjacencyTemplate(dataObj);
        }

        this.performSavingAction(
            action,
            this.dataService.getUserDeviceServerAdjacencies(this.user.id)
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
            .forEach((address) => {
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

                if (!server || (this.isAddressDirty && this.addresses.length > 0)) {
                    return;
                }

                this.isAddressDirty = false;
                this.addresses = server.healthcheckAddress ? [server.healthcheckAddress] : [];
            });
    }
}
