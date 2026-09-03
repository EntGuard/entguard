import { AddressPoolModel } from '../../../shared/models/address-pool.model';
import { LdapServerModel } from './ldap-template-server-model';

export class LdapTemplateModel {
    id: number;
    name: string;
    filter: string;
    isAdmin: boolean;
    mfaType: string;
    servers: LdapServerModel[];
    interfaceName: string;
    listenPort: string;
    mtu: string;
    dns: string[];
    addressPools: AddressPoolModel[];


    constructor(data = {}) {
        this.id = data['id'];
        this.name = data['name'];
        this.filter = data['filter'];
        this.isAdmin = data['is_admin'];
        this.mfaType = data['mfa_type'];
        this.servers = (data['servers'] || []).map(s => new LdapServerModel(s));
        this.interfaceName = data['iface_name'];
        this.listenPort = data['listen_port'];
        this.mtu = data['mtu'];
        this.dns = data['dns'] || [];
        this.addressPools = (data['address_pools'] || []).map(p => new AddressPoolModel(p));
    }

    setDataFromModel(template: LdapTemplateModel) {
        this.id = template.id;
        this.name = template.name;
        this.filter = template.filter;
        this.isAdmin = template.isAdmin;
        this.mfaType = template.mfaType;
        this.servers = template.servers;
        this.interfaceName = template.interfaceName;
        this.listenPort = template.listenPort;
        this.mtu = template.mtu;
        this.dns = template.dns;
        this.addressPools = template.addressPools;
    }
}
