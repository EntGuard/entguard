import { Component, Inject, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormControl, FormGroup, Validators } from '@angular/forms';
import { UserModel } from '../../../../../shared/models/user-model';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { UsersDataService } from '../../../../users/users-data.service';
import { AppUtilsService } from '../../../../../core/app-utils.service';
import { CommonFormDialogComponent } from '../../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { TranslateService } from '@ngx-translate/core';
import { takeUntil } from 'rxjs/operators';
import { Subject } from 'rxjs';

@Component({
    selector: 'app-user-dialog',
    templateUrl: './user-dialog.component.html',
    styleUrls: ['./user-dialog.component.scss'],
    standalone: false,
})
export class UserDialogComponent extends CommonFormDialogComponent implements OnInit, OnDestroy {
    protected componentDestroyed: Subject<void> = new Subject<void>();
    form: FormGroup;
    user: UserModel;
    showChars = false;
    editing = false;
    mfaTypes: Array<string>;
    mfaTypesError: any;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: UsersDataService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService
    ) {

        super(dialogRef, appUtilsService, translateService);
        this.user = data['user'] || new UserModel({});
        if (data['user']) {
            this.editing = true;
        }
    }

    ngOnInit(): void {
        this.dataService.getMfaTypesList().pipe(
            takeUntil(this.componentDestroyed))
            .subscribe(
                (res: Array<string>) => this.mfaTypes = res,
                error => this.mfaTypesError = error,
            );

        this.form = this.fb.group({
            name: [this.user.username, Validators.required],
            isAdmin: [this.user.isAdmin],
            showChars: [false],
            mfa_type: [this.user.mfaType],
            notification: [this.user.notification]
        });

        if (!this.user.username) {
            this.form.addControl('password', new FormControl('', Validators.required));
        }
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    submit() {
        const dataObj = {
            username: this.form.get('name').value,
            is_admin: !!this.form.get('isAdmin').value,
            mfa_type: this.form.get('mfa_type').value || '',
            notification: this.form.get('notification').value
        };
        let action1 = null;
        let action2 = null;

        if (this.editing) {
            action1 = this.dataService.patchUser(this.user.id, dataObj);
            action2 = this.dataService.getUser(this.user.id);
        } else {
            dataObj['password'] = this.form.get('password').value;
            action1 = this.dataService.postUser(dataObj);
            action2 = this.dataService.getUsersList();
        }

        this.performSavingAction(action1, action2);
    }

    close() {
        this.dialogRef.close();
    }

    toggleCharsShow() {
        this.showChars = !this.showChars;
    }
}
