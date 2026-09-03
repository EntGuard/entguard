export interface Server {
    id: string;
    name: string;
    endpoint?: string;
    description?: string;
}

export interface User {
    id: string;
    username: string;
    isAdmin: boolean;
    authType: string;
    mfaType: string;
}

export interface UserAdjacency {
    id: string;
    server: Server;
    user: User;
    userSideAllowedIPs: string[];
    presharedKey: string;
}

export class UserAdjacencyModel implements UserAdjacency {
    id: string;
    server: Server;
    user: User;
    userSideAllowedIPs: string[];
    presharedKey: string;

    constructor(data: Partial<UserAdjacency>) {
        this.id = data.id || '';
        this.server = data.server || {
            id: '',
            name: ''
        };
        this.user = data.user || {
            id: '',
            username: '',
            isAdmin: false,
            authType: '',
            mfaType: ''
        };
        this.userSideAllowedIPs = data.userSideAllowedIPs || [];
        this.presharedKey = data.presharedKey || '';
    }

    setDataFromModel(data: Partial<UserAdjacency>): void {
        Object.assign(this, data);
    }
}
