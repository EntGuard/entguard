import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../environments/environment';
import { map } from 'rxjs/operators';

@Injectable({
  providedIn: 'root'
})
export class ApiServiceService {
    revalidatingOperations = ['SPONSOR_APPROVAL', 'PRIV_OWNER_APPROVAL'];

    constructor(private httpClient: HttpClient) {
    }

    protected getListOfModels<T>(restURI: string, wrappingPropertyName: string, modelType: { new(value: any): T }): Observable<T[]> {
        return this.makeGetRequest(restURI)
            .pipe(
                map(
                    (objData: any) => {
                        if (objData[wrappingPropertyName]) {
                            return objData[wrappingPropertyName].map((modelData: any) => new modelType(modelData));
                        } else {
                            return [];
                        }
                    }
                ));
    }

    protected makeGetRequest(restUri: string): Observable<any> {
        return this.httpClient.get(
            environment.REST_BASE_URL + restUri,
            {
                headers: {'Content-Type': 'application/json'},
                withCredentials: true
            }
        );
    }

    protected makePostRequest(restUri: string, data: any): Observable<any> {
        return this.httpClient.post(
            environment.REST_BASE_URL + restUri,
            data,
            {
                headers: {'Content-Type': 'application/json'},
                withCredentials: true
            }
        );
    }






}
