import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { CommonFormDialogComponent } from '../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { ServerModel } from '../../../../shared/models/server-model';
import { ServersDataService } from '../../servers-data.service';
import { Patterns } from 'src/app/shared/validator-patterns';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-edit-server-dialog',
    templateUrl: './edit-server-dialog.component.html',
    styleUrls: ['./edit-server-dialog.component.scss'],
    standalone: false,
})
export class EditServerDialogComponent
    extends CommonFormDialogComponent
    implements OnInit {
    form: FormGroup;
    server: ServerModel;
    error: any;
    savingServer = false;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        protected appUtilsService: AppUtilsService,
        protected dataService: ServersDataService,
        private fb: FormBuilder,
        protected translateService: TranslateService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.server = data['server'];
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            name: [this.server.name, Validators.required],
            endpoint: [
                this.server.endpoint,
                [Validators.required, Validators.pattern(Patterns.IPv4noCIDR)],
            ],
            description: [this.server.description, Validators.maxLength(250)],
            healthcheckAddress: [
                this.server.healthcheckAddress,
                [Validators.pattern(Patterns.IPv4noCIDR)],
            ],
        });
    }

    saveServer() {
        const dataObj = {
            name: this.form.get('name').value,
            endpoint: this.form.get('endpoint').value,
            description: this.form.get('description').value,
            healthcheck_address: this.form.get('healthcheckAddress').value,
        };
        this.performSavingAction(
            this.dataService.updateServer(this.server.id, dataObj),
            this.dataService.getServer(this.server.id)
        );
    }

    close() {
        this.dialogRef.close();
    }
}
