import { Injectable } from '@angular/core';
import { DataModelsService } from '../../core/data-models.service';
import { HttpClient } from '@angular/common/http';
import { ServerModel } from '../../shared/models/server-model';
import { Observable } from 'rxjs';
import { VpnWireguardConfigModel } from '../../shared/models/vpn-wireguard-config-model';
import { ServerAdjacencyModel } from '../../shared/models/server-adjacency-model';
import { UserModel } from '../../shared/models/user-model';
import { UserAdjacencyModel } from '../../shared/models/user-adjacency-model';
import { ServerStatusModel } from '../../shared/models/server-status-model';
import { DeviceConfigurationModel } from '../../shared/models/device-configuration.model';

@Injectable({
    providedIn: 'root',
})
export class ServersDataService extends DataModelsService {
    constructor(httpClient: HttpClient) {
        super(httpClient);
    }

    getServersList(): Observable<ServerModel[]> {
        return this.getWrappedListOfModels('servers', ['servers'], ServerModel);
    }

    getServersStatuses(): Observable<ServerStatusModel[]> {
        return this.getWrappedListOfModels('servers/statuses', ['statuses'], ServerStatusModel);
    }

    getServer(id: number): Observable<ServerModel> {
        return this.getOneModel('servers/' + id, ServerModel);
    }

    postServer(serverData): Observable<void> {
        return this.post('servers', serverData);
    }

    updateServer(id, serverNewData): Observable<void> {
        return this.patch('servers/' + id, serverNewData);
    }

    deleteServer(id: number) {
        return this.delete('servers/' + id);
    }

    getServerVpnConfig(
        serverId: number
    ): Observable<VpnWireguardConfigModel> {
        return this.getOneModel(
            'servers/' + serverId + '/vpn/config',
            VpnWireguardConfigModel
        );
    }

    getServerAdjacencies(
        serverId: number
    ): Observable<ServerAdjacencyModel[]> {
        return this.getWrappedListOfModels(
            'vpn/adjacencies/servers/' + serverId,
            ['adjacencies'],
            ServerAdjacencyModel
        );
    }

    getUsersList(): Observable<UserModel[]> {
        return this.getWrappedListOfModels('users', ['users'], UserModel);
    }

    getDevicesList(): Observable<DeviceConfigurationModel[]> {
        return this.getWrappedListOfModels('device-configurations', ['device-configurations'], DeviceConfigurationModel);
    }

    // tslint:disable-next-line:ban-types
    postAdjacency(dataObj: Object): Observable<void> {
        return this.post('vpn/adjacencies', dataObj);
    }

    patchServerAdjacency(adjacencyId: number, dataObj: Object): Observable<void> {
        return this.patch(
            `vpn/adjacencies/${adjacencyId}`,
            dataObj
        );
    }

    deleteServerAdjacency(
        adjacency: ServerAdjacencyModel
    ) {
        return this.delete(
            'vpn/adjacencies/' + adjacency.id
        );
    }

    deleteUserAdjacency(adjacency: UserAdjacencyModel) {
        return this.delete(
            'vpn/adjacencies/' + adjacency.id
        );
    }

    patchServerConfiguration(serverId: number, dataObj: Object) {
        return this.patch('servers/' + serverId + '/vpn/config', dataObj);
    }
}
