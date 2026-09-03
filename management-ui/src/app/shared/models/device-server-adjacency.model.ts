export class DeviceServerAdjacencyModel {
    id: number;
    server: any;
    device: any;
    allowedIps: string[];
    otherSideAllowedIps: string[];
    presharedKey: string;
    persistentKeepalive: string;

    constructor(data: any = {}) {
        this.id = data.id;
        this.server = data.server;
        this.device = data.device;
        this.allowedIps = data.config?.allowed_ips || [];
        this.otherSideAllowedIps = data.config?.other_side_allowed_ips || [];
        this.presharedKey = data.config?.preshared_key || '';
        this.persistentKeepalive = data.config?.persistent_keepalive || '';
    }
}
