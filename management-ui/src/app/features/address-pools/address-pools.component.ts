import { Component, OnDestroy, OnInit } from '@angular/core';
import { ColDef } from 'ag-grid-community';
import { Subject } from 'rxjs';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { UsersDataService } from '../users/users-data.service';
import { AppUtilsService } from '../../core/app-utils.service';
import { AddressPoolModel } from '../../shared/models/address-pool.model';
import { MatDialog } from '@angular/material/dialog';
import { AddressPoolDialogComponent } from './address-pool-dialog/address-pool-dialog.component';
import { TranslateService } from '@ngx-translate/core';

@Component({
    selector: 'app-address-pools',
    templateUrl: './address-pools.component.html',
    styleUrls: ['./address-pools.component.scss'],
    standalone: false,
})
export class AddressPoolsComponent implements OnInit, OnDestroy {
    loading = true;
    addressPools: AddressPoolModel[] = [];

    addressPoolsCols: ColDef[] = [
        {
            colId: 'name',
            field: 'name',
            headerName: 'address-pools.cellName',
            filter: true,
        },
        {
            colId: 'addressRange',
            field: 'addressRange',
            headerName: 'address-pools.cellAddressRange',
            filter: true,
        },
        {
            colId: 'description',
            field: 'description',
            headerName: 'address-pools.cellDescription',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            resizable: true,
            maxWidth: 170,
            headerName: '',
        },
    ];

    private componentDestroyed: Subject<void> = new Subject<void>();

    constructor(
        private dataService: UsersDataService,
        private appUtils: AppUtilsService,
        private dialog: MatDialog,
        private translateService: TranslateService
    ) { }

    ngOnInit(): void {
        this.translateHeaderNames(this.addressPoolsCols);
        this.loadAddressPools();
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    private translateHeaderNames(cols: ColDef[]): void {
        this.translateService.stream('address-pools').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            cols.forEach(col => {
                if (col.headerName) {
                    col.headerName = this.translateService.instant(col.headerName);
                }
            });
        });
    }

    loadAddressPools(): void {
        this.loading = true;
        this.dataService.getAddressPools()
            .pipe(
                finalize(() => {
                    this.loading = false;
                }),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.addressPools = result;
                },
                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
    }

    onAddAddressPoolClick(): void {
        const dialogRef = this.dialog.open(AddressPoolDialogComponent, {
            width: '40rem',
            data: {}
        });

        dialogRef.afterClosed().subscribe(result => {
            if (result) {
                this.loadAddressPools();
            }
        });
    }

    onEditAddressPoolClick(pool: AddressPoolModel): void {
        const dialogRef = this.dialog.open(AddressPoolDialogComponent, {
            width: '40rem',
            data: { pool }
        });

        dialogRef.afterClosed().subscribe(result => {
            if (result) {
                this.loadAddressPools();
            }
        });
    }

    onDeleteAddressPoolClick(pool: AddressPoolModel): void {
        this.translateService.stream('address-pools.confirmRemoveAddressPool')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText + pool.name + '?',
                        this.dataService.deleteAddressPool(pool.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(result => {
                if (result) {
                    this.loadAddressPools();
                }
            });
    }
}
