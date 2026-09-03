import { Component, OnDestroy } from '@angular/core';
import { MatDialogRef } from '@angular/material/dialog';
import { AppUtilsService } from '../../../core/app-utils.service';
import { Observable, Subject } from 'rxjs';
import { takeUntil } from 'rxjs/operators';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-common-form-dialog',
    templateUrl: './common-form-dialog.component.html',
    styleUrls: ['./common-form-dialog.component.scss'],
    standalone: false,
})
export class CommonFormDialogComponent implements OnDestroy {
    protected dialogRef: MatDialogRef<any>;
    protected applicationUtilsService: AppUtilsService;

    protected componentDestroyed: Subject<void> = new Subject<void>();

    savingProgress = false;

    error: any;

    constructor(
        dialogRef: MatDialogRef<any>,
        appUtilsService: AppUtilsService,
        protected translateService: TranslateService
    ) {
        this.dialogRef = dialogRef;
        this.applicationUtilsService = appUtilsService;
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    /**
     * @param savingObservable observable for the action to be performed as the 'save' action
     * @param afterSaveObservable observable for the action to be performed after 'save' action and return the result of it
     * (designed for refreshing data after performing main action)
     */
    performSavingAction(
        savingObservable: Observable<any>,
        afterSaveObservable?: Observable<any>
    ): void {
        this.initProgress();
        savingObservable.pipe(takeUntil(this.componentDestroyed)).subscribe(
            () => {
                if (afterSaveObservable) {
                    afterSaveObservable
                        .pipe(takeUntil(this.componentDestroyed))
                        .subscribe(
                            (data) => this.dialogRef.close(data),
                            (errAfterSave) => this.printError(errAfterSave)
                        );
                } else {
                    this.dialogRef.close(true);
                }
            },
            (err) => this.printError(err)
        );
    }

    protected printError(err: any) {
        if (err && err['error'] && err['error']['message']) {
            this.error = err['error']['message'];
        } else {
            this.error = err['message'] || err;
            if (!this.error) {
                this.translateService.stream('shared.unknownError').pipe(takeUntil(this.componentDestroyed))
                    .subscribe((translatedValue) => {
                        this.error = translatedValue;
                    });
            }
        }
        this.savingProgress = false;
    }

    protected initProgress() {
        this.error = null;
        this.savingProgress = true;
    }
}
