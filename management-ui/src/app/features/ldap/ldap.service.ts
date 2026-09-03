import { Injectable } from '@angular/core';
import { DataModelsService } from '../../core/data-models.service';
import { HttpClient } from '@angular/common/http';
import { LdapConfigModel } from './models/ldap-config-model';
import { LdapTemplateModel } from './models/ldap-template-model';
import { Observable } from 'rxjs';
import { LdapSyncModel } from './models/ldap-sync-model';
import { LdapInterfaceModel } from './models/ldap-template-interface-model';
import { LdapServerModel } from './models/ldap-template-server-model';
import { map } from 'rxjs/operators';

@Injectable({
    providedIn: 'root',
})
export class LDAPService extends DataModelsService {
    constructor(httpClient: HttpClient) {
        super(httpClient);
    }

    getMfaTypesList(): Observable<Array<string>> {
        return this.customGet('/mfa/types').pipe(map((res) => res['types']));
    }

    getTemplates(): Observable<LdapTemplateModel[]> {
        return this.getWrappedListOfModels(
            'ldap-templates',
            ['ldap_templates'],
            LdapTemplateModel
        );
    }

    getTemplate(id: number): Observable<LdapTemplateModel> {
        return this.getOneModel('ldap-templates/' + id + '/servers', LdapTemplateModel);
    }

    addTemplate(input: any): Observable<any> {
        return this.post('ldap-templates', input);
    }

    patchTemplate(id: number, input: any): Observable<any> {
        return this.patch('ldap-templates/' + id, input);
    }

    deleteTemplate(id: number): Observable<LdapTemplateModel> {
        return this.delete('ldap-templates/' + id);
    }

    getTemplateServers(templateId: number): Observable<LdapServerModel[]> {
        return this.getWrappedListOfModels(
            'ldap-templates/' + templateId + '/servers',
            ['ldap_template_servers'],
            LdapServerModel
        );
    }

    addTemplateServer(templateId: number, input: any): Observable<any> {
        return this.post('ldap-templates/' + templateId + '/servers', input);
    }

    patchTemplateServer(
        templateId: number,
        oldServerId: number,
        input: any
    ): Observable<any> {
        return this.patch(
            'ldap-templates/' + templateId + '/servers/' + oldServerId,
            input
        );
    }

    deleteTemplateServer(
        templateId: number,
        serverId: number
    ): Observable<LdapServerModel> {
        return this.delete(
            'ldap-templates/' + templateId + '/servers/' + serverId
        );
    }

    getConfigs(): Observable<LdapConfigModel[]> {
        return this.getWrappedListOfModels(
            'ldap',
            ['ldap_configs'],
            LdapConfigModel
        );
    }

    getSyncStatus(): Observable<LdapSyncModel> {
        return this.getOneModel('ldap/sync/status', LdapSyncModel);
    }

    resync(): Observable<LdapSyncModel> {
        return this.post('ldap/sync/resync', null);
    }

    getConfig(id: string): Observable<LdapConfigModel> {
        return this.getOneModel('ldap/' + id, LdapConfigModel);
    }

    deleteConfig(id: string): Observable<LdapConfigModel> {
        return this.delete('ldap/' + id);
    }

    addConfig(input: any): Observable<any> {
        return this.post('ldap', input);
    }

    patchConfig(id: string, input: any): Observable<any> {
        return this.patch('ldap/' + id, input);
    }
}
