export class WireGuardInterfaceModel {
    name: string;
    privateKey: string;
    publicKey: string;
    addresses: string[];
    listenPort: string;
    dns: string[];
    mtu: string;
    persistentKeepalive: string;

    constructor(data: any = {}) {
        this.name = data['name'] || 'eg0';
        this.privateKey = data['private_key'];
        this.publicKey = data['public_key'];
        this.addresses = data['addresses'] || [];
        this.listenPort = data['listen_port'];
        this.dns = data['dns'] || [];
        this.mtu = data['mtu'];
        this.persistentKeepalive = data['persistent_keepalive'];
    }
}
