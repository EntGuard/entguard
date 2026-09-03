export class MfaCrlModel {
    id: number;
    issuer: string;
    serialNumbers: string[];

    constructor(data = {}) {
        this.id = data['id'];
        this.issuer = data['issuer'];
        this.serialNumbers = data['serial_numbers'];
    }
}
