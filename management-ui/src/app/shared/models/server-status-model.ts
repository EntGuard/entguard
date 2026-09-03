interface ServerStatus {
    id: number;
    name: string;
    vpn_server: {
        status: string;
        message_key: string;
        message_args: string[];
    };
    healthcheck: {
        status: string;
        message_key: string;
        message_args: string[];
    };
}

export class ServerStatusModel {
    id: number;
    name: string;
    vpnServer: {
        status: string;
        messageKey: string;
        messageArgs: string[];
    };
    healthcheck: {
        status: string;
        messageKey: string;
        messageArgs: string[];
    };

    constructor(data: ServerStatus) {
        this.setData(data);
    }

    setData(data: ServerStatus) {
        this.id = data.id;
        this.name = data.name;
        this.vpnServer = {
            status: data.vpn_server.status,
            messageKey: data.vpn_server.message_key,
            messageArgs: data.vpn_server.message_args,
        };
        this.healthcheck = {
            status: data.healthcheck.status,
            messageKey: data.healthcheck.message_key,
            messageArgs: data.healthcheck.message_args,
        };
    }

    setDataFromModel(data: ServerStatusModel) {
        this.id = data.id;
        this.name = data.name;
        this.vpnServer = data.vpnServer;
        this.healthcheck = data.healthcheck;
    }
}
