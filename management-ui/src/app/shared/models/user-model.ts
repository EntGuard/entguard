export class UserModel {
    id: number;
    username: string;
    password?: string;
    isAdmin: boolean;
    mfaType: string;
    notification: string;
    lastEditTime: string;
    authType: string;
    totpSecret?: string;

    constructor(data = {}) {
        this.id = data['id'];
        this.username = data['username'];
        this.password = data['password'];
        this.isAdmin = data['is_admin'];
        this.mfaType = data['mfa_type'] === 'none' ? '' : data['mfa_type'];
        this.notification = data['notification'];
        this.lastEditTime = this.normalizeDate(data['last_edit_time']);
        this.authType = data['auth_type'];
    }

    private normalizeDate(date: string): string | null {
        return date && date !== '0001-01-01T00:00:00Z' ? date : null;
    }

    setDataFromModel(user: UserModel) {
        this.id = user.id;
        this.username = user.username;
        this.password = user.password;
        this.isAdmin = user.isAdmin;
        this.mfaType = user.mfaType;
        this.notification = user.notification;
        this.lastEditTime = this.normalizeDate(user.lastEditTime);
        this.authType = user.authType;
    }
}
