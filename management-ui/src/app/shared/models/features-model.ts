export class FeaturesModel {
    ldap: boolean;


    constructor(data = {}) {
        this.ldap = data['ldap'];
    }
}
