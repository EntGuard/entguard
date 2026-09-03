export class TotpSecretModel {
    totpKey: string;

    constructor(data = {}) {
        this.totpKey = data['totp_key'];
    }
}
