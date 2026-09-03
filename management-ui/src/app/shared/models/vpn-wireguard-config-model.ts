import { WireGuardInterfaceModel } from './wire-guard-interface-model';

export class VpnWireguardConfigModel {
    interface: WireGuardInterfaceModel;

    constructor(data = {}) {
        this.interface = new WireGuardInterfaceModel(data);
    }
}
