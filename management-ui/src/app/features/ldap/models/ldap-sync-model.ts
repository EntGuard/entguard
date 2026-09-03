export class LdapSyncModel {
    startedAt: Date;
    inProgress: boolean;
    syncDuration: string;
    ldapServersConfigured: number;
    ldapServersProcessed: number;
    usersRetrieved: number;
    usersCreated: number;
    usersUpdated: number;
    usersDeleted: number;
    relationsCreated: number;
    relationsUpdated: number;
    relationsDeleted: number;
    errorCount: number;
    errors: string[];

    constructor(data = {}) {
        this.startedAt = data['started_at'];
        this.inProgress = data['in_progress'];
        this.syncDuration = data['sync_duration'];
        this.ldapServersConfigured = data['ldap_servers_configured'];
        this.ldapServersProcessed = data['ldap_servers_processed'];
        this.usersRetrieved = data['users_retrieved'];
        this.usersCreated = data['users_created'];
        this.usersUpdated = data['users_updated'];
        this.usersDeleted = data['users_deleted'];
        this.relationsCreated = data['relations_created'];
        this.relationsUpdated = data['relations_updated'];
        this.relationsDeleted = data['relations_deleted'];
        this.errorCount = data['error_count'] || 0;
        this.errors = data['error_list'];
    }
}
