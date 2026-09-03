export class ServerModel {
    id: number;
    name: string;
    endpoint: string;
    healthcheckAddress: string;
    description: string;


    constructor(data = {}) {
        this.setData(data);
    }


    setData(data: {}) {
        this.id = data['id'];
        this.name = data['name'];
        this.endpoint = data['endpoint'];
        this.healthcheckAddress = data['healthcheck_address'];
        this.description = data['description'];
    }

    setDataFromModel(data: ServerModel) {
        this.id = data.id;
        this.name = data.name;
        this.endpoint = data.endpoint;
        this.healthcheckAddress = data.healthcheckAddress;
        this.description = data.description;
    }
}
