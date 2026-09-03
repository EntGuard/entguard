import { Component, OnDestroy, OnInit } from '@angular/core';
import { Subject } from 'rxjs';
import { finalize, takeUntil } from 'rxjs/operators';
import { LDAPService } from 'src/app/features/ldap/ldap.service';
import { LdapSyncModel } from 'src/app/features/ldap/models/ldap-sync-model';
import { MatDialog } from '@angular/material/dialog';
import { LDAPErrorsComponent } from './ldap-errors/ldap-errors.component';

@Component({
    selector: 'app-ldap-synchronization',
    templateUrl: './ldap-synchronization.component.html',
    styleUrls: ['./ldap-synchronization.component.scss'],
    standalone: false,
})
export class LdapSynchronizationComponent implements OnInit, OnDestroy {
    private componentDestroyed: Subject<void> = new Subject<void>();
    loading = true;
    syncStatus: LdapSyncModel;
    syncError: any;
    syncProgress = false;

    constructor(private dataService: LDAPService, private dialog: MatDialog) { }

    ngOnInit(): void {
        this.refreshSyncStatus();
    }

    private refreshSyncStatus() {
        this.dataService
            .getSyncStatus()
            .pipe(
                finalize(() => (this.loading = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (status: LdapSyncModel) => {
                    this.syncStatus = status;
                    if (status.inProgress) {
                        setTimeout(() => {
                            return this.refreshSyncStatus();
                        }, 500);
                    } else {
                        this.syncError = null;
                    }
                },
                (err) => {
                    this.syncError = err;
                }
            );
    }

    onSyncClick() {
        this.syncError = null;
        this.syncProgress = true;
        this.dataService
            .resync()
            .pipe(
                finalize(() => (this.syncProgress = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                () => {
                    if (this.syncStatus) {
                        this.syncStatus.inProgress = true;
                    }
                    this.refreshSyncStatus();
                },
                (err) => {
                    this.syncError = err;
                }
            );
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    onShowErrorClick() {
        this.dialog.open(LDAPErrorsComponent, { data: { errors: this.syncStatus.errors }, width: '60%' });

    }
}
