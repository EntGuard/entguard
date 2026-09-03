import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../environments/environment';
import { map } from 'rxjs/operators';

@Injectable({
    providedIn: 'root',
})
export class DataModelsService {
    headers = new HttpHeaders({
        'Content-Type': 'application/json',
    });

    constructor(protected httpClient: HttpClient) { }

    /**
     * Can be used when as a response is expected array of data objects, which should be converted to models objects
     */
    protected getListOfModels<T>(
        restURI: string,
        modelType: new (value: any) => T
    ): Observable<T[]> {
        return this.customGet(restURI).pipe(
            map(
                // tslint:disable-next-line:ban-types
                (arrayData: Object[]) =>
                    arrayData.map(
                        (modelData: Object) => new modelType(modelData)
                    )
            )
        );
    }

    protected getWrappedListOfModels<T>(
        restURI: string,
        wrappingObjProperties: string[],
        modelType: new (value: any) => T
    ): Observable<T[]> {
        return this.customGet(restURI).pipe(
            map((arrayDataWrapper: any) => {
                if (arrayDataWrapper) {
                    let wrapperSubObj = arrayDataWrapper;
                    wrappingObjProperties.forEach((wrapperProp: string) => {
                        if (wrapperSubObj.hasOwnProperty(wrapperProp)) {
                            wrapperSubObj = wrapperSubObj[wrapperProp];
                        }
                    });
                    if (wrapperSubObj && wrapperSubObj.length > 0) {
                        // tslint:disable-next-line:ban-types
                        return wrapperSubObj.map(
                            (modelData: Object) => new modelType(modelData)
                        );
                    } else {
                        return [];
                    }
                } else {
                    return [];
                }
            })
        );
    }

    customGet(restURI: string): Observable<any> {
        return this.httpClient.get(environment.REST_BASE_URL + restURI, {
            withCredentials: true,
            headers: this.headers,
        });
    }

    /**
     * Can be used when as a response is expected array of data objects, which should be converted to models objects
     */
    // tslint:disable-next-line:ban-types
    protected getListOfModelsWithPostMethod<T>(
        restURI: string,
        modelType: new (value: any) => T,
        dataObject: Object
    ): Observable<T[]> {
        return this.httpClient
            .post(environment.REST_BASE_URL + restURI, dataObject, {
                withCredentials: true,
            })
            .pipe(
                map(
                    // tslint:disable-next-line:ban-types
                    (arrayData: Object[]) =>
                        arrayData.map(
                            (modelData: Object) => new modelType(modelData)
                        )
                )
            );
    }

    /**
     * Can be used when as a response is expected single data object, which should be converted to model object
     */
    protected getOneModel<T>(
        restURI: string,
        modelType: new (value: any) => T
    ): Observable<T> {
        return this.httpClient
            .get(environment.REST_BASE_URL + restURI, {
                withCredentials: true,
                headers: this.headers,
            })
            .pipe(
                // tslint:disable-next-line:ban-types
                map((modelData: Object[]) => new modelType(modelData))
            );
    }

    protected getOneWrappedModel<T>(
        restURI: string,
        wrappingObjProperties: string[],
        modelType: new (value: any) => T
    ): Observable<T> {
        return this.httpClient
            .get(environment.REST_BASE_URL + restURI, { withCredentials: true })
            .pipe(
                map((arrayDataWrapper: any) => {
                    if (arrayDataWrapper) {
                        let wrapperSubObj = arrayDataWrapper;
                        wrappingObjProperties.forEach((wrapperProp: string) => {
                            if (wrapperSubObj.hasOwnProperty(wrapperProp)) {
                                wrapperSubObj = wrapperSubObj[wrapperProp];
                            }
                        });
                        return new modelType(wrapperSubObj);
                    } else {
                        return null;
                    }
                })
            );
    }

    /**
     * Common http post with added credentials
     */
    // tslint:disable-next-line:ban-types
    protected post<T>(restURI: string, dataObject: Object): Observable<any> {
        return this.httpClient.post(
            environment.REST_BASE_URL + restURI,
            dataObject,
            { withCredentials: true, headers: this.headers }
        );
    }

    protected patch<T>(restURI: string, dataObject: Object): Observable<any> {
        return this.httpClient.patch(
            environment.REST_BASE_URL + restURI,
            dataObject,
            { withCredentials: true, headers: this.headers }
        );
    }

    /**
     * Common http delete with added credentials
     */
    // tslint:disable-next-line:ban-types
    protected delete<T>(restURI: string): Observable<any> {
        return this.httpClient.delete(environment.REST_BASE_URL + restURI, {
            withCredentials: true,
            headers: this.headers,
        });
    }
}
