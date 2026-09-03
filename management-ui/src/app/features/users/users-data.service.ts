import { Injectable } from '@angular/core';
import { DataModelsService } from '../../core/data-models.service';
import { HttpClient } from '@angular/common/http';
import { Observable, of } from 'rxjs';
import { UserModel } from '../../shared/models/user-model';
import { VpnWireguardConfigModel } from '../../shared/models/vpn-wireguard-config-model';
import { UserAdjacencyModel } from '../../shared/models/user-adjacency-model';
import { TotpSecretModel } from '../../shared/models/totop-secret-model';
import { map } from 'rxjs/operators';
import { AddressPoolModel } from '../../shared/models/address-pool.model';
import { DeviceTemplateModel } from '../../shared/models/device-template.model';
import { AdjacencyTemplateModel } from '../../shared/models/adjacency-template.model';
import { DeviceModel } from '../../shared/models/device.model';
import { DeviceConfigurationModel } from '../../shared/models/device-configuration.model';
import { DeviceServerAdjacencyModel } from '../../shared/models/device-server-adjacency.model';

@Injectable({
    providedIn: 'root',
})
export class UsersDataService extends DataModelsService {
    constructor(httpClient: HttpClient) {
        super(httpClient);
    }

    getUsersList(): Observable<UserModel[]> {
        return this.getWrappedListOfModels('users', ['users'], UserModel);
    }

    getUser(id: number): Observable<UserModel> {
        return this.getOneModel('users/' + id, UserModel);
    }

    getUserVpnConfig(userId: number): Observable<VpnWireguardConfigModel> {
        return this.getOneModel(
            'users/' + userId + '/vpn/config',
            VpnWireguardConfigModel
        );
    }

    getUserDeviceTemplate(userId: number): Observable<DeviceTemplateModel> {
        return this.getOneModel(
            `users/${userId}/device-template`,
            DeviceTemplateModel
        );
    }

    getUserAdjacencies(userId: number): Observable<UserAdjacencyModel[]> {
        return this.getWrappedListOfModels(
            'vpn/adjacencies/users/' + userId,
            ['adjacencies'],
            UserAdjacencyModel
        );
    }

    // tslint:disable-next-line:ban-types
    postAdjacency(dataObj: Object): Observable<void> {
        return this.post('vpn/adjacencies', dataObj);
    }

    patchUserAdjacency(
        adjacencyId: number,
        dataObj: Object
    ): Observable<void> {
        return this.patch(
            `vpn/adjacencies/${adjacencyId}`,
            dataObj
        );
    }

    // Adjacency Templates API methods
    getUserAdjacencyTemplates(userId: number): Observable<AdjacencyTemplateModel[]> {
        return this.getWrappedListOfModels(
            `vpn/adjacency-templates/users/${userId}`,
            ['adjacency_templates'],
            AdjacencyTemplateModel
        );
    }

    createAdjacencyTemplate(dataObj: Object): Observable<void> {
        return this.post('vpn/adjacency-templates', dataObj);
    }

    updateAdjacencyTemplate(id: number, dataObj: Object): Observable<void> {
        return this.patch(`vpn/adjacency-templates/${id}`, dataObj);
    }

    deleteAdjacencyTemplate(id: number): Observable<void> {
        return this.delete(`vpn/adjacency-templates/${id}`);
    }

    postUser(userData: Object): Observable<void> {
        return this.post('users', userData);
    }

    patchUser(id: number, userData: Object): Observable<void> {
        return this.patch('users/' + id, userData);
    }

    getMfaTypesList(): Observable<Array<string>> {
        return this.customGet('/mfa/types').pipe(map((res) => res['types']));
    }

    getTotpSecret(userId: number): Observable<TotpSecretModel> {
        return this.getOneWrappedModel(
            'users/' + userId + '/totp',
            ['secrets'],
            TotpSecretModel
        );
    }

    createTotpSecret(userId: number): Observable<TotpSecretModel> {
        return this.post('users/' + userId + '/totp', {}).pipe(
            map((res) => new TotpSecretModel(res['secrets']))
        );
    }

    patchTotpSecret(userId: number): Observable<TotpSecretModel> {
        return this.patch('users/' + userId + '/totp', {}).pipe(
            map((res) => new TotpSecretModel(res['secrets']))
        );
    }

    // tslint:disable-next-line:ban-types
    postUserConfiguration(userId: number, dataObj: Object) {
        return this.post('users/' + userId + '/vpn/config', dataObj);
    }

    patchUserConfiguration(userId: number, dataObj: Object) {
        return this.patch('users/' + userId + '/vpn/config', dataObj);
    }

    createUserDeviceTemplate(userId: number, dataObj: Object) {
        return this.post(`users/${userId}/device-template`, dataObj);
    }

    updateUserDeviceTemplate(userId: number, dataObj: Object) {
        return this.patch(`users/${userId}/device-template`, dataObj);
    }

    deleteUser(id: number): Observable<void> {
        return this.delete('users/' + id);
    }

    deleteUserVpnConfig(userId: number): Observable<void> {
        return this.delete('users/' + userId + '/vpn/config');
    }

    deleteUserDeviceTemplate(userId: number): Observable<void> {
        return this.delete(`users/${userId}/device-template`);
    }

    getAddressPools(): Observable<AddressPoolModel[]> {
        return this.getWrappedListOfModels(
            'address-pools',
            ['address_pools'],
            AddressPoolModel
        );
    }

    postAddressPool(data: any): Observable<void> {
        return this.post('address-pools', data);
    }

    patchAddressPool(id: number, data: any): Observable<void> {
        return this.patch('address-pools/' + id, data);
    }

    deleteAddressPool(id: number): Observable<void> {
        return this.delete('address-pools/' + id);
    }

    getUserDeviceTemplates(userId: number): Observable<DeviceTemplateModel[]> {
        return this.getWrappedListOfModels(
            `users/${userId}/device-templates`,
            ['device-templates'],
            DeviceTemplateModel
        );
    }

    getDevices(userId: number): Observable<DeviceModel[]> {
        return this.getWrappedListOfModels(
            `device-templates/${userId}/devices`,
            ['devices'],
            DeviceModel
        );
    }

    autogenDevice(deviceTemplateId: number): Observable<DeviceConfigurationModel> {
        return this.customGet(`device-templates/${deviceTemplateId}/devices/autogen`)
            .pipe(map(data => new DeviceConfigurationModel(data)));
    }

    createDevice(deviceTemplateId: number, deviceData: Object): Observable<any> {
        return this.post(`device-templates/${deviceTemplateId}/devices`, deviceData);
    }

    updateDevice(deviceId: number, deviceData: Object): Observable<void> {
        return this.patch(`devices/${deviceId}`, deviceData);
    }

    deleteDevice(deviceId: number): Observable<void> {
        return this.delete(`devices/${deviceId}`);
    }

    syncDevices(deviceTemplateId: number): Observable<any> {
        return this.post(`device-templates/${deviceTemplateId}/devices/resync`, {});
    }

    getDeviceServerAdjacencies(
        deviceId: number
    ): Observable<DeviceServerAdjacencyModel[]> {
        return this.getWrappedListOfModels(
            `devices/${deviceId}/adjacencies`,
            ['adjacencies'],
            DeviceServerAdjacencyModel
        );
    }

    getUserDeviceServerAdjacencies(
        userId: number
    ): Observable<DeviceServerAdjacencyModel[]> {
        return this.getWrappedListOfModels(
            `vpn/adjacencies/user/${userId}`,
            ['adjacencies'],
            DeviceServerAdjacencyModel
        );
    }

    syncDeviceServers(userId: number): Observable<any> {
        return this.post(`users/${userId}/device-adjacencies/resync`, {});
    }

    getDeviceConfigurations(): Observable<DeviceModel[]> {
        return this.getWrappedListOfModels(
            'device-configurations',
            ['device-configurations'],
            DeviceModel
        );
    }

    postDeviceServerAdjacency(adjacencyData: Object): Observable<void> {
        return this.post(`vpn/adjacencies`, adjacencyData);
    }

    patchDeviceServerAdjacency(adjacencyId: number, adjacencyData: Object): Observable<void> {
        return this.patch(`vpn/adjacencies/${adjacencyId}`, adjacencyData);
    }

    deleteDeviceServerAdjacency(adjacencyId: number): Observable<void> {
        return this.delete(`vpn/adjacencies/${adjacencyId}`);
    }

    getDeviceServerAdjacency(deviceId: number, adjacencyId: number): Observable<DeviceServerAdjacencyModel> {
        return this.getOneModel(`devices/${deviceId}/adjacencies/${adjacencyId}`, DeviceServerAdjacencyModel);
    }

    postDeviceTemplate(userId: number, templateData: Object): Observable<void> {
        return this.post(`users/${userId}/device-templates`, templateData);
    }

    patchDeviceTemplate(userId: number, templateId: number, templateData: Object): Observable<void> {
        return this.patch(`users/${userId}/device-templates/${templateId}`, templateData);
    }

    deleteDeviceTemplate(userId: number, templateId: number): Observable<void> {
        return this.delete(`users/${userId}/device-templates/${templateId}`);
    }

    getDeviceTemplate(userId: number, templateId: number): Observable<DeviceTemplateModel> {
        return this.getOneModel(`users/${userId}/device-templates/${templateId}`, DeviceTemplateModel);
    }
}
