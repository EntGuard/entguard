import { AddressPoolModel } from '../../../shared/models/address-pool.model';

export class LdapInterfaceModel {
    name: string;
    addresses: AddressPoolModel[];
    listenPort: string;
    dns: string[];
    mtu: string;


    constructor(data = {}) {
        this.name = data['iface_name'] || '';
        this.addresses = (data['iface_addrs'] || []).map(addr =>
            new AddressPoolModel(addr)
        );
        this.listenPort = data['listen_port'];
        this.dns = data['dns'] || [];
        this.mtu = data['mtu'];
    }
}
