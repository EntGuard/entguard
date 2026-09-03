export class LdapConfigModel {
    id: string;
    priority: number;
    host: string;
    port: number;
    baseDn: string;
    bindDn: string;
    userListFilter: string;
    usernameAttr: string;
    uidAttr: string;
    useTLS: boolean;
    fqdn: string;
    template: number;
    caCert: string;


    constructor(data = {}) {
        this.id = data['id'];
        this.priority = data['priority'];
        this.host = data['host'];
        this.port = data['port'];
        this.baseDn = data['base_dn'];
        this.bindDn = data['bind_dn'];
        this.userListFilter = data['user_list_filter'];
        this.usernameAttr = data['username_attribute'];
        this.uidAttr = data['uid_attribute'];
        this.useTLS = data['use_tls'];
        this.fqdn = data['fqdn'];
        this.template = data['template_id'];
        this.caCert = data['ca_cert'];
    }
}
