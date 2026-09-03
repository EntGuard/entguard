import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { DataModelsService } from '../../core/data-models.service';
import { CertModel } from '../../shared/models/cert-model';
import { MfaCaModel } from '../../shared/models/mfa-ca-model';
import { MfaCrlModel } from '../../shared/models/mfa-crl-model';

@Injectable({
    providedIn: 'root',
})
export class MfaCertificatesService extends DataModelsService {
    constructor(httpClient: HttpClient) {
        super(httpClient);
    }

    addMfaCertificateCa(dataObj: CertModel): Observable<void> {
        return this.post('mfa/cert/ca', dataObj);
    }

    loadMfaCertificatesCa(): Observable<MfaCaModel[]> {
        return this.getWrappedListOfModels(
            'mfa/cert/ca',
            ['ca_list'],
            MfaCaModel
        );
    }

    removeMfaCertificateCa(id: number): Observable<void> {
        return this.delete('mfa/cert/ca/' + id);
    }

    addMfaCertificateCrl(dataObj: CertModel): Observable<void> {
        return this.post('mfa/cert/crl', dataObj);
    }

    loadMfaCertificatesCrl(): Observable<MfaCrlModel[]> {
        return this.getWrappedListOfModels(
            'mfa/cert/crl',
            ['crl_list'],
            MfaCrlModel
        );
    }

    removeMfaCertificateCrl(id: number): Observable<void> {
        return this.delete('mfa/cert/crl/' + id);
    }
}
