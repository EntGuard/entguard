import { Component, Inject } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { DeviceModel } from '../../../../../shared/models/device.model';

@Component({
    selector: 'app-device-info-dialog',
    templateUrl: './device-info-dialog.component.html',
    styleUrls: ['./device-info-dialog.component.scss'],
    standalone: false,
})
export class DeviceInfoDialogComponent {
    device: DeviceModel;
    showPrivateKey = false;
    deviceInfoEntries: Array<{ label: string; value: string }> = [];

    constructor(
        @Inject(MAT_DIALOG_DATA) public data: { device: DeviceModel },
        private dialogRef: MatDialogRef<DeviceInfoDialogComponent>
    ) {
        this.device = data.device;
        this.deviceInfoEntries = this.mapDeviceInformation(this.device?.deviceInformation);
    }

    closeDialog(): void {
        this.dialogRef.close();
    }

    togglePrivateKeyShow(): void {
        this.showPrivateKey = !this.showPrivateKey;
    }

    private mapDeviceInformation(info?: string | null): Array<{ label: string; value: string }> {
        if (!info) {
            return [];
        }

        // Parse the pre-formatted multiline string from the backend
        // Format: "OS:          iOS\nOS Version:  17.2\n..."
        return info
            .split('\n')
            .map((line) => line.trim())
            .filter((line) => line.length > 0)
            .map((line) => {
                const separatorIndex = line.indexOf(':');
                const label = separatorIndex > -1 ? line.slice(0, separatorIndex).trim() : line.trim();
                const value = separatorIndex > -1 ? line.slice(separatorIndex + 1).trim() : '';

                return {
                    label,
                    value: value !== '' ? value : '-',
                };
            });
    }
}
