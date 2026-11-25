import { Component, Inject, ChangeDetectionStrategy } from '@angular/core';
import { MAT_DIALOG_DATA } from '@angular/material/dialog';

@Component({
    selector: 'app-http-error-dialog',
    templateUrl: './http-error-dialog.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    standalone: false,
})
export class HttpErrorDialogComponent {
    constructor(@Inject(MAT_DIALOG_DATA) public data: any) { }
}
