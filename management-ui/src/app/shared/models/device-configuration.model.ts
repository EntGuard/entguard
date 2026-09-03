export class DeviceConfigurationModel {
    id: number;
    description: string;
    privateKey: string;
    publicKey: string;
    addresses: string[];
    externalDeviceId: string;
    sessionId: string;
    lastTimeConnected: string;

    constructor(data: any = {}) {
        this.id = data['id'];
        this.description = data['description'];
        this.privateKey = data['private_key'] || data['privateKey'];
        this.publicKey = data['public_key'] || data['publicKey'];
        this.addresses = data['addresses'] || [];
        this.externalDeviceId = data['device_id'] || data['externalDeviceId'] || data['deviceId'];
        this.sessionId = data['session_id'] || data['sessionId'];
        this.lastTimeConnected = this.normalizeDate(data['last_time_connected'] || data['lastTimeConnected']);
    }

    private normalizeDate(date: string): string | null {
        return date && date !== '0001-01-01T00:00:00Z' ? date : null;
    }

    setDataFromModel(data: DeviceConfigurationModel): void {
        this.id = data.id;
        this.description = data.description;
        this.privateKey = data.privateKey;
        this.publicKey = data.publicKey;
        this.addresses = data.addresses;
        this.externalDeviceId = data.externalDeviceId;
        this.sessionId = data.sessionId;
        this.lastTimeConnected = this.normalizeDate(data.lastTimeConnected);
    }
}
