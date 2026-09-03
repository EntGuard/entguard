import { Component, OnInit, Inject, OnDestroy } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Observable, Subject } from 'rxjs';
import { takeUntil } from 'rxjs/operators';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-confirm-dialog',
    templateUrl: './confirm-dialog.component.html',
    styleUrls: ['./confirm-dialog.component.scss'],
    standalone: false,
})
export class ConfirmDialogComponent implements OnDestroy {
    confirmMsg = '';
    warningMsg = '';

    actionToBePerformed: Observable<any>;
    action2ToBePerformed: Observable<any>;

    error = '';
    savingProgress = false;
    private componentDestroyed: Subject<void> = new Subject<void>();

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: any,
        private dialogRef: MatDialogRef<ConfirmDialogComponent>,
        private translateService: TranslateService
    ) {
        this.confirmMsg = data['confirmMsg'];
        this.warningMsg = data['warningMsg'] || '';
        this.actionToBePerformed = data['actionToBePerformed'];
        this.action2ToBePerformed = data['action2ToBePerformed'];
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    submit() {
        this.initProgress();

        if (this.actionToBePerformed) {
            this.actionToBePerformed
                .pipe(takeUntil(this.componentDestroyed))
                .subscribe(
                    () => {
                        if (this.action2ToBePerformed) {
                            this.action2ToBePerformed
                                .pipe(takeUntil(this.componentDestroyed))
                                .subscribe(
                                    (data) => this.dialogRef.close(data),
                                    (errAfterSave) =>
                                        this.printError(errAfterSave)
                                );
                        } else {
                            this.dialogRef.close(true);
                        }
                    },
                    (err) => this.printError(err)
                );
        } else {
            this.dialogRef.close(true);
        }
    }

    protected initProgress() {
        this.error = null;
        this.savingProgress = true;
    }

    protected printError(err: any) {
        if (err && err['error'] && err['error']['message']) {
            this.error = err['error']['message'];
        } else {
            this.error = err['message'];
            if (!this.error) {
                this.translateService.stream('shared.unknownError').pipe(takeUntil(this.componentDestroyed))
                    .subscribe((translatedValue) => {
                        this.error = translatedValue;
                    });
            }
        }
        this.savingProgress = false;
    }
}
