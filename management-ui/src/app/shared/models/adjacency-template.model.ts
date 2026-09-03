import { ServerModel } from './server-model';

export class AdjacencyTemplateModel {
    id: number;
    server: ServerModel;
    allowedIps: string[];
    usePresharedKey: boolean;

    constructor(data: any = {}) {
        this.id = data['id'] || null;
        this.server = data['server'] ? new ServerModel(data['server']) : null;
        this.allowedIps = data['template_config'] ? data['template_config']['client_side_allowed_ips'] : [];
        this.usePresharedKey = data['template_config']
            ? !!data['template_config']['use_preshared_key']
            : false;
    }
}
