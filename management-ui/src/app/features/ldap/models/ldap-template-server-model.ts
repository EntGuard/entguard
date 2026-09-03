export class LdapServerModel {
    id: number;
    name: string;
    allowedIps: string[];
    usePresharedKey: boolean;


    constructor(data = {}) {
        this.id = data['server_id'];
        this.name = data['server_name'];
        this.allowedIps = data['client_side_allowed_ips'] || [];
        this.usePresharedKey = !!data['use_preshared_key'];
    }
}
