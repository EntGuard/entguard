export class DeviceModel {
    id: number;
    description: string;
    privateKey: string;
    publicKey: string;
    addresses: string[];
    deviceId: string;
    sessionId: string;
    lastTimeConnected: string;
    deviceInformation: string;

    constructor(data: any = {}) {
        this.id = data['id'];
        this.description = data['description'];
        this.privateKey = data['private_key'];
        this.publicKey = data['public_key'];
        this.addresses = data['addresses'] || [];
        this.deviceId = data['device_id'];
        this.sessionId = data['session_id'];
        this.lastTimeConnected = this.normalizeDate(data['last_time_connected']);
        this.deviceInformation = data['device_information'];
    }

    private normalizeDate(date: string): string | null {
        return date && date !== '0001-01-01T00:00:00Z' ? date : null;
    }
}
