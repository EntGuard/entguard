export class AddressPoolModel {
    id: number;
    name: string;
    addressStart: string;
    addressEnd: string;
    netmask: number;
    description: string;

    constructor(data = {}) {
        this.id = data['id'] || 0;
        this.name = data['name'] || '';
        this.addressStart = data['start_addr'] || data['addressStart'] || '';
        this.addressEnd = data['end_addr'] || data['addressEnd'] || '';
        this.netmask = data['net_mask'] || data['netmask'] || 0;
        this.description = data['description'] || '';
    }

    // Helper method to get the formatted address range for display
    get addressRange(): string {
        if (this.addressStart && this.addressEnd && this.netmask !== null && this.netmask !== undefined) {
            return `${this.addressStart} - ${this.addressEnd} /${this.netmask}`;
        }
        return '';
    }

    // Helper method to get a formatted tooltip with labels
    get tooltipText(): string {
        let tooltip = 'Address Pool:\n' + this.addressRange;
        if (this.description) {
            tooltip += '\n\nDescription:\n' + this.description;
        }
        return tooltip;
    }
}
