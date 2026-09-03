import { Component, Inject } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

@Component({
    selector: 'app-sync-result-dialog',
    templateUrl: './sync-result-dialog.component.html',
    styleUrls: ['./sync-result-dialog.component.scss'],
    standalone: false
})
export class SyncResultDialogComponent {
    constructor(
        public dialogRef: MatDialogRef<SyncResultDialogComponent>,
        @Inject(MAT_DIALOG_DATA) public data: any
    ) { }

    onClose(): void {
        this.dialogRef.close();
    }

    formatKey(key: string): string {
        return key.split('_').map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(' ');
    }

    isArray(value: any): boolean {
        return Array.isArray(value);
    }
}
