import { DeviceConfigurationModel } from './device-configuration.model';

export class ServerAdjacencyModel {
    id: number;
    device: DeviceConfigurationModel;
    publicKey: string;
    allowedIps: string[];
    otherSideAllowedIps: string[];
    presharedKey: string;
    persistentKeepalive: string;

    constructor(data = {}) {
        this.id = data['id'];
        this.device = data['device'] ? new DeviceConfigurationModel(data['device']) : null;
        this.publicKey = data['config'] ? data['config']['public_key'] : '';
        this.allowedIps = data['config'] ? data['config']['allowed_ips'] : [];
        this.otherSideAllowedIps = data['config'] ? data['config']['other_side_allowed_ips'] : [];
        this.presharedKey = data['config'] ? data['config']['preshared_key'] : '';
        this.persistentKeepalive = data['config'] ? data['config']['persistent_keepalive'] : '';
    }
}
