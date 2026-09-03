import { Component, OnDestroy, OnInit } from '@angular/core';
import { MatDialog } from '@angular/material/dialog';
import { Observable, Subject } from 'rxjs';
import { MfaCaModel } from '../../shared/models/mfa-ca-model';
import { MfaCrlModel } from '../../shared/models/mfa-crl-model';
import { MfaCertificatesService } from './mfa-certificates.service';
import { MfaCertificateDialogComponent } from './mfa-certificate-dialog/mfa-certificate-dialog.component';
import { MfaCertificatesIssuerDialogComponent } from './mfa-certificates-issuer-dialog/mfa-certificates-issuer-dialog.component';
import { ColDef } from 'ag-grid-community';
import { AppUtilsService } from '../../core/app-utils.service';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-mfa-certificates',
    templateUrl: './mfa-certificates.component.html',
    styleUrls: ['./mfa-certificates.component.scss'],
    standalone: false,
})
export class MfaCertificatesComponent implements OnInit, OnDestroy {
    private componentDestroyed: Subject<void> = new Subject<void>();
    mfaCertificatesCa: MfaCaModel[];
    mfaCertificatesCrl: MfaCrlModel[];
    loadingCaCertificates: boolean = true;
    loadingCrlCertificates: boolean = true;

    caCertificatesCols: ColDef[] = [
        {
            colId: 'id',
            field: 'id',
            headerName: 'mfa-certificates.cellID',
            filter: true,
        },
        {
            colId: 'commonName',
            field: 'commonName',
            headerName: 'mfa-certificates.cellName',
            filter: true,
        },
        {
            colId: 'notBefore',
            field: 'notBefore',
            headerName: 'mfa-certificates.cellNotBefore',
            filter: true,
        },
        {
            colId: 'notAfter',
            field: 'notAfter',
            headerName: 'mfa-certificates.cellNotAfter',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,
            resizable: false,
            pinned: 'right',
            minWidth: 190,
            maxWidth: 190,
        },
    ];
    crlCertificatesCols: ColDef[] = [
        {
            colId: 'id',
            field: 'id',
            headerName: 'mfa-certificates.cellID',
            filter: true,
        },
        {
            colId: 'serialNumbers',
            field: 'serialNumbers',
            headerName: 'mfa-certificates.cellSerialNumbers',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,
            resizable: false,
            pinned: 'right',
            minWidth: 190,
            maxWidth: 190,
        },
    ];

    constructor(
        private dialog: MatDialog,
        private mfaCertificatesService: MfaCertificatesService,
        private appUtils: AppUtilsService,
        private translateService: TranslateService
    ) { }

    ngOnInit(): void {
        this.translateCertificatesHeaderNames(this.caCertificatesCols);
        this.translateCertificatesHeaderNames(this.crlCertificatesCols);
        this.loadMfaCertificatesCa();
        this.loadMfaCertificatesCrl();
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    private translateCertificatesHeaderNames(certificateCols: ColDef[]): void {
        this.translateService.stream('mfa-certificates').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            certificateCols.forEach(certificateCol => {
                if (certificateCol.headerName) {
                    certificateCol.headerName = this.translateService.instant(certificateCol.headerName);
                }
            });
        });
    }

    onAddMfaCertificateClick(type: string) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(MfaCertificateDialogComponent, {
                width: '400px',
                data: {
                    certificateType: type,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((res) => {
            if (res) {
                if (type === 'CA') {
                    this.mfaCertificatesCa = res;
                } else {
                    this.mfaCertificatesCrl = res;
                }
            }
        });
    }

    onRemoveMfaCertificateClick(certificate: MfaCaModel | MfaCrlModel, type: string) {
        let loadCertificates: Observable<MfaCaModel[]> | Observable<MfaCrlModel[]> = this.mfaCertificatesService.loadMfaCertificatesCa();
        let deleteCertificate = this.mfaCertificatesService.removeMfaCertificateCa(certificate.id);
        let messageStream = this.translateService.stream('mfa-certificates.confirmRemoveCACertif');

        if (type === 'CRL') {
            loadCertificates = this.mfaCertificatesService.loadMfaCertificatesCrl();
            deleteCertificate = this.mfaCertificatesService.removeMfaCertificateCrl(certificate.id);
            messageStream = this.translateService.stream('mfa-certificates.confirmRemoveCRLCertif');
        }

        messageStream.pipe(
            switchMap((message) =>
                this.appUtils.showConfirmation(message, deleteCertificate, loadCertificates)
            ),
            takeUntil(this.componentDestroyed)
        ).subscribe((result) => {
            if (result) {
                if (type === 'CA') {
                    this.mfaCertificatesCa = result;
                } else {
                    this.mfaCertificatesCrl = result;
                }
            }
        });
    }

    loadMfaCertificatesCa() {
        this.loadingCaCertificates = true;
        this.mfaCertificatesService
            .loadMfaCertificatesCa()
            .pipe(finalize(() => (this.loadingCaCertificates = false)))
            .subscribe(
                (res: MfaCaModel[]) => (this.mfaCertificatesCa = res),
                (error) => this.appUtils.handleHttpError(error)
            );
    }

    loadMfaCertificatesCrl() {
        this.loadingCrlCertificates = true;
        this.mfaCertificatesService
            .loadMfaCertificatesCrl()
            .pipe(finalize(() => (this.loadingCrlCertificates = false)))
            .subscribe(
                (res: MfaCrlModel[]) => (this.mfaCertificatesCrl = res),
                (error) => this.appUtils.handleHttpError(error)
            );
    }

    onShowIssuerDialogClick(crlCertificate: MfaCrlModel) {
        this.dialog
            .open(MfaCertificatesIssuerDialogComponent, {
                data: {
                    data: crlCertificate.issuer,
                },
            })
            .afterClosed();
    }
}
