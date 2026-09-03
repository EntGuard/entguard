export class LoggedUserModel {
    username: string;
    password: string;
    jwt: string;

    constructor(data = {}) {
        this.username = data['username'];
        this.password = data['password'];
        this.jwt = data['jwt'];
    }
}
