import { Component, OnDestroy, OnInit, ViewChild } from '@angular/core';
import { ServerModel } from '../../../shared/models/server-model';
import { ColDef } from 'ag-grid-community';
import { MatDialog } from '@angular/material/dialog';
import { AddServerDialogComponent } from './add-server-dialog/add-server-dialog.component';
import { interval, Observable, Subject, timer } from 'rxjs';
import { ServersDataService } from '../servers-data.service';
import { delayWhen, finalize, retryWhen, startWith, switchMap, takeUntil, tap } from 'rxjs/operators';
import { AppUtilsService } from '../../../core/app-utils.service';
import { VpnWireguardConfigModel } from '../../../shared/models/vpn-wireguard-config-model';
import { ServerConfigDialogComponent } from './server-config-dialog/server-config-dialog.component';
import { ServerAdjacencyModel } from '../../../shared/models/server-adjacency-model';
import { ServerAdjacencyDialogComponent } from './server-adjacency-dialog/server-adjacency-dialog.component';
import { ActivatedRoute, Router } from '@angular/router';
import { EditServerDialogComponent } from './edit-server-dialog/edit-server-dialog.component';
import { AppAgGridComponent } from '../../../shared/components/ag-grid/app-ag-grid.component';
import { TranslateService } from '@ngx-translate/core';
import { ServerStatusModel } from 'src/app/shared/models/server-status-model';

@Component({
    selector: 'app-servers',
    templateUrl: './servers-list.component.html',
    styleUrls: ['./servers-list.component.scss'],
    standalone: false,
})
export class ServersListComponent implements OnInit, OnDestroy {

    @ViewChild('serversGrid') serversGrid: AppAgGridComponent;

    loading = true;
    loadingServerConfig = false;
    serverCols: ColDef[] = [
        {
            colId: 'name',
            field: 'name',
            headerName: 'servers-list.cellServerName',
            filter: true,
            maxWidth: 420,
        },
        {
            colId: 'vpnServerStatus',
            field: 'vpnServerStatus',
            headerName: 'servers-list.cellServerStatus',
            maxWidth: 200,
            flex: 0,
        },
        {
            colId: 'healthcheckStatus',
            field: 'healthcheckStatus',
            headerName: 'servers-list.cellHealthcheckStatus',
            maxWidth: 200,
            flex: 0,
        },
        {
            colId: 'description',
            field: 'description',
            headerName: 'servers-list.cellServerDesc',
            flex: 2,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,
            resizable: false,
            pinned: 'right',
            minWidth: 190,
            maxWidth: 190,
        }

    ];
    servers: ServerModel[] = [];
    serverStatuses: ServerStatusModel[] = [];
    selectedServer: ServerModel;
    serverConfig: VpnWireguardConfigModel;

    loadingServerAdjacencies = false;

    serverAdjacencies: ServerAdjacencyModel[] = [];

    private componentDestroyed: Subject<void> = new Subject<void>();
    serverDeviceCols: ColDef[] = [
        {
            colId: 'device',
            field: 'user.username',
            headerName: 'server-adjacency-dialog.placeholderDevice',
            filter: true,
        },
        {
            colId: 'allowedIps',
            field: 'allowedIps',
            headerName: 'server-adjacency-dialog.labelAllowedIps',
            filter: true,
        },
        {
            colId: 'otherSideAllowedIps',
            field: 'otherSideAllowedIps',
            headerName: 'server-adjacency-dialog.labelOtherSideAllowedIps',
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
        }
    ];

    constructor(
        private dialog: MatDialog,
        private dataService: ServersDataService,
        private appUtils: AppUtilsService,
        private route: ActivatedRoute,
        private router: Router,
        private translateService: TranslateService,
    ) {
    }

    ngOnInit(): void {
        this.translateServerHeaderNames(this.serverCols);
        this.translateServerHeaderNames(this.serverDeviceCols);
        this.loadServers().subscribe(
            () => {
                this.route.params.subscribe(
                    params => {
                        if (params['server']) {
                            const s = this.servers.find(server => '' + server.id === params['server']);
                            if (s) {
                                this.setSelectedServer(s);
                            }
                        }
                    }
                );
                this.pollServerStatuses();
            }
        );
    }

    private translateServerHeaderNames(servers: ColDef[]): void {
        this.translateService.stream('servers-list').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            servers.forEach(server => {
                if (server.headerName) {
                    server.headerName = this.translateService.instant(server.headerName);
                }
            });
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }


    loadSelectedServerVpnConfig(): void {
        this.loadingServerConfig = true;
        this.dataService.getServerVpnConfig(this.selectedServer.id).pipe(
            finalize(() => {
                this.loadingServerConfig = false;
            }),
            takeUntil(this.componentDestroyed)
        ).subscribe(
            result => {
                this.serverConfig = result;
            },
            err => {
                this.appUtils.handleHttpError(err);
            }
        );
    }

    loadServers(): Observable<void> {
        const res: Subject<void> = new Subject<void>();
        this.loading = true;
        this.dataService.getServersList().pipe(
            finalize(() => {
                this.loading = false;
                res.next();
            }),
            takeUntil(this.componentDestroyed)
        ).subscribe(
            result => {
                this.servers = result;
            },
            err => {
                this.appUtils.handleHttpError(err);
            }
        );
        return res.asObservable();
    }

    pollServerStatuses(): void {
        interval(5000).pipe(
            startWith(0),
            switchMap(() => this.dataService.getServersStatuses().pipe(
                retryWhen(errors => errors.pipe(
                    tap(() => this.serverStatuses = []),
                    delayWhen(() => timer(5000))
                ))
            )),
            takeUntil(this.componentDestroyed)
        ).subscribe(
            result => {
                this.serverStatuses = result;
            }
        );
    }

    getServerStatus(serverId: number): ServerStatusModel {
        return this.serverStatuses.find(status => status.id === serverId);
    }

    onAddServerClick() {
        const dialogRef = this.dialog.open(AddServerDialogComponent, { disableClose: true });
        dialogRef.componentInstance.onAddServer.subscribe((res: ServerModel[]) => {
            this.servers = res;
        });

        dialogRef.afterClosed().subscribe(response => {
            if (response && response.configureServerId) {
                this.router.navigate([`servers/${response.configureServerId}`]);
            }
        });
    }


    onDeleteServerClick(server: ServerModel) {
        this.translateService.stream('servers-list.confirmRemoveServer')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText + server.name + '?',
                        this.dataService.deleteServer(server.id),
                        this.dataService.getServersList()
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(result => {
                if (result) {
                    this.servers = result;
                    if (this.selectedServer) {
                        this.onDeselectServer();
                    }
                }
            });
    }

    setSelectedServer(server: ServerModel) {
        this.selectedServer = server;
        this.loadSelectedServerVpnConfig();
        this.loadSelectedServerAdjacencies();
    }

    onDeselectServer() {
        this.selectedServer = null;
        this.serverConfig = null;
        this.serverAdjacencies = [];
        this.loadServers();
    }

    onAddInterfaceClick() {
        const dialogAfterClose: Observable<any> = this.dialog.open(ServerConfigDialogComponent, {
            data: {
                server: this.selectedServer
            },
            width: '65%'
        }).afterClosed();

        dialogAfterClose.subscribe(response => {
            if (response) {
                this.serverConfig = response;
            }
        });
    }

    onEditInterfaceClick(config) {
        const dialogAfterClose: Observable<any> = this.dialog.open(ServerConfigDialogComponent, {
            data: {
                server: this.selectedServer,
                config
            },
            width: '65%'
        }).afterClosed();

        dialogAfterClose.subscribe(response => {
            if (response) {
                this.serverConfig = response;
            }
        });
    }

    private loadSelectedServerAdjacencies() {
        this.loadingServerAdjacencies = true;
        this.dataService.getServerAdjacencies(this.selectedServer.id).pipe(
            finalize(() => {
                this.loadingServerAdjacencies = false;
            }),
            takeUntil(this.componentDestroyed)
        ).subscribe(
            result => {
                this.serverAdjacencies = result;
            },

            err => {
                this.appUtils.handleHttpError(err);
            }
        );
    }

    onAddDeviceClick() {

        const dialogAfterClose: Observable<any> = this.dialog.open(ServerAdjacencyDialogComponent, {
            width: '400px',
            height: 'auto',
            data: {
                server: this.selectedServer
            }
        }).afterClosed();

        dialogAfterClose.subscribe(response => {
            if (response) {
                this.serverAdjacencies = response;
            }
        });
    }

    onEditServerAdjacencyClick(adjacency: ServerAdjacencyModel) {
        const dialogAfterClose: Observable<any> = this.dialog.open(ServerAdjacencyDialogComponent, {
            width: '420px',
            data: {
                server: this.selectedServer,
                adjacency
            }
        }).afterClosed();

        dialogAfterClose.subscribe(response => {
            if (response) {
                this.serverAdjacencies = response;
            }
        });
    }

    onDeleteServerAdjacencyClick(row: any) {
        this.translateService.stream('servers-list.confirmRemoveDevice')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText,
                        this.dataService.deleteServerAdjacency(row),
                        this.dataService.getServerAdjacencies(this.selectedServer.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(result => {
                if (result) {
                    this.serverAdjacencies = result;
                }
            });
    }

    onEditServerClick(selectedServer: ServerModel) {
        const dialogAfterClose: Observable<any> = this.dialog.open(EditServerDialogComponent, {
            width: '420px',
            height: 'auto',
            data: {
                server: selectedServer
            }
        }).afterClosed();

        dialogAfterClose.subscribe(response => {
            if (response) {
                selectedServer.setDataFromModel(response);
                if (!this.selectedServer) {
                    this.serversGrid.refreshView();
                }
            }
        });
    }
}
