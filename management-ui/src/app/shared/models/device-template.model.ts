import { AddressPoolModel } from './address-pool.model';

export class DeviceTemplateModel {
    id: number;
    interfaceName: string;
    addressPools: AddressPoolModel[];
    listenPort?: string;
    dns: string[];
    mtu?: string;

    constructor(data: any = {}) {
        this.id = data['id'];
        this.interfaceName = data['interface_name'] || data['interfaceName'] || data['name'];
        this.addressPools = (data['address_pools'] || data['addressPools'] || [])
            .map((pool: any) => pool instanceof AddressPoolModel ? pool : new AddressPoolModel(pool));
        this.listenPort = data['listen_port'] || data['listenPort'];
        this.dns = data['dns'] || [];
        this.mtu = data['mtu'];
    }
}
