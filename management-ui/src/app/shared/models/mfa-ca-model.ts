export class MfaCaModel {
    id: number;
    commonName: string;
    notAfter: string;
    notBefore: string;

    constructor(data = {}) {
        this.id = data['id'];
        this.commonName = data['common_name'];
        this.notAfter = data['not_after'];
        this.notBefore = data['not_before'];
    }
}
