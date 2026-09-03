import { Component, Inject, OnInit } from '@angular/core';
import { CommonFormDialogComponent } from '../../../../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import {
    FormBuilder,
    FormControl,
    FormGroup,
    Validators,
} from '@angular/forms';
import { ServerModel } from '../../../../../shared/models/server-model';
import { ServerAdjacencyModel } from '../../../../../shared/models/server-adjacency-model';
import { UserModel } from '../../../../../shared/models/user-model';
import { COMMA, ENTER, SEMICOLON, SPACE } from '@angular/cdk/keycodes';
import { Observable } from 'rxjs';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { ServersDataService } from '../../../../servers/servers-data.service';
import { AppUtilsService } from '../../../../../core/app-utils.service';
import { finalize, map, startWith, takeUntil } from 'rxjs/operators';
import { AuthService } from '../../../../../core/auth.service';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-user-password-dialog',
    templateUrl: './user-password-dialog.component.html',
    styleUrls: ['./user-password-dialog.component.scss'],
    standalone: false,
})
export class UserPasswordDialogComponent
    extends CommonFormDialogComponent
    implements OnInit {
    form: FormGroup;
    userId: string;

    showChars = false;

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        protected dialogRef: MatDialogRef<any>,
        private fb: FormBuilder,
        private dataService: AuthService,
        protected appUtilsService: AppUtilsService,
        protected translateService: TranslateService
    ) {
        super(dialogRef, appUtilsService, translateService);
        this.userId = data['userId'];
    }

    ngOnInit(): void {
        this.form = this.fb.group({
            password: ['', Validators.required],
        });
    }

    submit() {
        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }
        this.performSavingAction(
            this.dataService.patchPassword(
                this.userId,
                this.form.get('password').value
            )
        );
    }

    close() {
        this.dialogRef.close();
    }

    toggleCharsShow() {
        this.showChars = !this.showChars;
    }
}
