import {
    Component,
    ElementRef,
    OnDestroy,
    OnInit,
    ViewChild,
} from '@angular/core';
import { ColDef } from 'ag-grid-community';
import { UserModel } from '../../../../shared/models/user-model';
import { DeviceTemplateModel } from '../../../../shared/models/device-template.model';
import { UserAdjacencyModel } from '../../../../shared/models/user-adjacency-model';
import { DeviceConfigurationModel } from '../../../../shared/models/device-configuration.model';
import { UserAdjacencyModel as NewUserAdjacencyModel } from '../../../../shared/models/user-adjacency-new.model';
import { AddressPoolModel } from '../../../../shared/models/address-pool.model';
import { AdjacencyTemplateModel } from '../../../../shared/models/adjacency-template.model';
import { Observable, of, Subject, forkJoin } from 'rxjs';
import { MatDialog } from '@angular/material/dialog';
import { UsersDataService } from '../../../users/users-data.service';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { UserDialogComponent } from './user-dialog/user-dialog.component';
import { UserConfigDialogComponent } from './user-config-dialog/user-config-dialog.component';
import { UserAdjacencyDialogComponent } from './user-adjacency-dialog/user-adjacency-dialog.component';
import { ServersDataService } from '../../../servers/servers-data.service';
import { ActivatedRoute, Router } from '@angular/router';
import { LoggedUserModel } from '../../../../shared/models/logged-user-model';
import { AuthService } from '../../../../core/auth.service';
import { AppAgGridComponent } from '../../../../shared/components/ag-grid/app-ag-grid.component';
import { UserPasswordDialogComponent } from './user-password-dialog/user-password-dialog.component';
import { DeviceDialogComponent } from './device-dialog/device-dialog.component';
import { DeviceServerDialogComponent } from './device-server-dialog/device-server-dialog.component';
import { DeviceInfoDialogComponent } from './device-info-dialog/device-info-dialog.component';
import { TotpSecretModel } from '../../../../shared/models/totop-secret-model';
import { Clipboard } from '@angular/cdk/clipboard';
import { TranslateService } from '@ngx-translate/core';
import { SyncResultDialogComponent } from '../../../../shared/dialogs/sync-result-dialog/sync-result-dialog.component';

@Component({
    selector: 'app-users-list',
    templateUrl: './users-list.component.html',
    styleUrls: ['./users-list.component.scss'],
    standalone: false,
})
export class UsersListComponent implements OnInit, OnDestroy {
    @ViewChild('usersGrid') usersGrid: AppAgGridComponent;
    @ViewChild('searchInput') searchInput: ElementRef;

    loading = true;
    loadingDeviceTemplate = false;
    loadingUserTotpSecret = false;
    userCols: ColDef[] = [
        {
            colId: 'name',
            field: 'username',
            headerName: 'user-list.cellUsername',
            filter: true,
        },
        {
            colId: 'isAdmin',
            field: 'isAdmin',
            headerName: 'user-list.cellIsAdmin',
            filter: true,
            cellDataType: 'text',
        },
        {
            colId: 'authType',
            field: 'authType',
            headerName: 'user-list.cellAuthType',
            filter: true,
        },
        {
            colId: 'mfaType',
            field: 'mfaType',
            headerName: 'user-list.cellMFAType',
            filter: true,
        },
        {
            colId: 'notification',
            field: 'notification',
            headerName: 'user-list.cellNotification',
            filter: true,
        },
        {
            colId: 'lastEditTime',
            field: 'lastEditTime',
            headerName: 'user-list.cellLastEditTime',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,
            minWidth: 190,
        },
    ];
    users: UserModel[] = [];
    selectedUser: UserModel;
    deviceTemplate: DeviceTemplateModel;
    loadingUserAdjacencies: boolean = false;
    showTotpSecretChars: boolean = false;
    userAdjacencies: AdjacencyTemplateModel[] = [];
    currentUser: LoggedUserModel;
    error: any;
    selectedTabIndex: number = 0;

    // Address pools for displaying interface addresses as chips
    addressPools: AddressPoolModel[] = [];
    userInterfaceAddressPools: AddressPoolModel[] = [];
    loadingAddressPools: boolean = false;

    // New properties for devices
    devices: any[] = [];
    loadingDevices: boolean = false;
    deviceServers: any[] = [];
    loadingDeviceServers: boolean = false;

    private componentDestroyed: Subject<void> = new Subject<void>();
    userServersCols: ColDef[] = [
        {
            colId: 'server',
            field: 'server.name',
            headerName: 'user-list.cellServer',
            filter: true,
        },
        {
            colId: 'allowedIps',
            field: 'allowedIps',
            headerName: 'user-list.cellAllowedIPs',
            filter: true,
        },
        {
            colId: 'usePresharedKey',
            field: 'usePresharedKey',
            headerName: 'user-list.cellUsePresharedKey',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,
            minWidth: 140,
            maxWidth: 140,
        },
    ];

    // Column definitions for devices
    devicesCols: ColDef[] = [
        {
            colId: 'id',
            field: 'id',
            headerName: 'user-list.cellID',
            filter: true,
        },
        {
            colId: 'description',
            field: 'description',
            headerName: 'user-list.cellDescription',
            filter: true,
        },
        {
            colId: 'addresses',
            field: 'addresses',
            headerName: 'user-list.cellAddresses',
            filter: true,
        },
        {
            colId: 'publicKey',
            field: 'publicKey',
            headerName: 'user-list.cellPublicKey',
            filter: true,
        },
        {
            colId: 'sessionId',
            field: 'sessionId',
            headerName: 'user-list.cellSessionID',
            filter: true,
        },
        {
            colId: 'lastTimeConnected',
            field: 'lastTimeConnected',
            headerName: 'user-list.cellLastTimeConnected',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,
            minWidth: 190,
            maxWidth: 190,
        },
    ];

    // Column definitions for device servers
    deviceServersCols: ColDef[] = [
        {
            colId: 'server',
            field: 'server.name',
            headerName: 'user-list.cellServer',
            filter: true,
        },
        {
            colId: 'device',
            field: 'device.id',
            headerName: 'user-list.cellDeviceId',
            filter: true,
        },
        {
            colId: 'allowedIps',
            field: 'allowedIps',
            headerName: 'user-list.cellClientSideAllowedIPs',
            filter: true,
        },
        {
            colId: 'otherSideAllowedIps',
            field: 'otherSideAllowedIps',
            headerName: 'user-list.cellServerSideAllowedIPs',
            filter: true,
        },
        {
            colId: 'persistentKeepalive',
            field: 'persistentKeepalive',
            headerName: 'user-list.cellPersistentKeepalive',
            filter: true,
        },
        {
            colId: 'presharedKey',
            field: 'presharedKey',
            headerName: 'user-list.cellPresharedKey',
            filter: true,
        },
        {
            colId: 'actions',
            filter: false,
            sortable: false,

            minWidth: 140,
            maxWidth: 140,
        },
    ];

    constructor(
        private dialog: MatDialog,
        private dataService: UsersDataService,
        private serversDataService: ServersDataService,
        private appUtils: AppUtilsService,
        private route: ActivatedRoute,
        private router: Router,
        private authService: AuthService,
        private clipboard: Clipboard,
        private translateService: TranslateService
    ) { }

    ngOnInit(): void {
        this.translateUserHeaderNames(this.userCols);
        this.translateUserHeaderNames(this.userServersCols);
        this.translateUserHeaderNames(this.devicesCols);
        this.translateUserHeaderNames(this.deviceServersCols);
        this.authService.currentUser
            .pipe(takeUntil(this.componentDestroyed))
            .subscribe((user) => {
                this.currentUser = user;
            });

        // Load address pools for displaying interface addresses as chips
        this.loadAddressPools();

        this.loadUsers().subscribe(() => {
            this.route.params.subscribe((params) => {
                if (params['user']) {
                    const user = this.users.find(
                        (u) => '' + u.id === params['user']
                    );
                    if (user) {
                        this.setSelectedUser(user);
                    }
                }
            });

            // Listen for query parameters to set the correct tab
            this.route.queryParams.subscribe((queryParams) => {
                const tabMapping: { [key: string]: number } = {
                    'devices': 0,
                    'templates': 1
                };

                if (queryParams['tab'] && tabMapping[queryParams['tab']] !== undefined) {
                    this.selectedTabIndex = tabMapping[queryParams['tab']];
                }
            });
        });
    }

    private translateUserHeaderNames(users: ColDef[]): void {
        this.translateService.stream('users-list').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            users.forEach(user => {
                if (user.headerName) {
                    user.headerName = this.translateService.instant(user.headerName);
                }
            });
        });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    reloadSelectedUser(): void {
        this.dataService
            .getUser(this.selectedUser.id)
            .pipe(
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.setSelectedUser(result)
                },
                (err) => {
                    this.error = err;
                }
            );
    }

    reloadAllData(): void {
        this.loadUsers().subscribe(() => {
            if (this.selectedUser) {
                const updatedUser = this.users.find(u => u.id === this.selectedUser.id);
                if (updatedUser) {
                    this.setSelectedUser(updatedUser);
                } else {
                    this.onDeselectUser();
                }
            }
        });
    }

    loadSelectedUserDeviceTemplate(): void {
        this.loadingDeviceTemplate = true;
        this.dataService
            .getUserDeviceTemplate(this.selectedUser.id)
            .pipe(
                finalize(() => {
                    this.loadingDeviceTemplate = false;
                }),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    if (result) {
                        this.deviceTemplate = result;
                        if (this.addressPools.length > 0) {
                            this.userInterfaceAddressPools = this.deviceTemplate.addressPools || [];
                        }
                        // Load devices after device template is loaded (uses deviceTemplate.id)
                        this.loadSelectedUserDevices();
                    } else {
                        this.deviceTemplate = undefined as any;
                        this.userInterfaceAddressPools = [];
                        this.devices = [];
                        this.deviceServers = [];
                    }
                },
                (err) => {
                    this.error = err;
                    this.deviceTemplate = undefined as any;
                    this.userInterfaceAddressPools = [];
                    this.devices = [];
                    this.deviceServers = [];
                }
            );
    }

    private loadAddressPools(): void {
        this.loadingAddressPools = true;
        this.dataService.getAddressPools()
            .pipe(
                finalize(() => this.loadingAddressPools = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (pools) => {
                    this.addressPools = pools;
                    // If device template is already loaded, update the interface addresses directly
                    if (this.deviceTemplate && this.deviceTemplate.addressPools) {
                        this.userInterfaceAddressPools = this.deviceTemplate.addressPools;
                    }
                },
                (err) => this.appUtils.handleHttpError(err)
            );
    }

    loadUsers(): Observable<void> {
        const res: Subject<void> = new Subject<void>();
        this.loading = true;
        this.dataService
            .getUsersList()
            .pipe(
                finalize(() => {
                    this.loading = false;
                    res.next();
                }),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.users = result;
                },
                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
        return res.asObservable();
    }

    onAddUserClick() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(UserDialogComponent, {
                data: {
                    user: null,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.users = response;
            }
        });
    }

    onClearSearchClick() {
        this.usersGrid.clearQuickSearchBar();
        this.searchInput.nativeElement.value = '';
    }

    onEditUserClick(selectedUser: UserModel) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(UserDialogComponent, {
                data: {
                    user: selectedUser,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                selectedUser.setDataFromModel(response);
                if (!this.selectedUser) {
                    this.usersGrid.refreshView();
                } else {
                    this.loadSelectedUserTotpSecret();
                }
            }
        });
    }

    onDeleteUserClick(user: UserModel) {
        this.translateService.stream('user-list.confirmRemoveUser')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText + user.username + '?',
                        this.dataService.deleteUser(user.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(result => {
                if (result) {
                    this.reloadAllData();
                }
            });
    }

    setSelectedUser(user: UserModel) {
        this.selectedUser = user;
        this.loadSelectedUserDeviceTemplate();
        this.loadSelectedUserAdjacencies();
        this.loadSelectedUserTotpSecret();
    }

    onDeselectUser() {
        this.selectedUser = undefined as any;
        this.deviceTemplate = undefined as any;
        this.userAdjacencies = [];
        this.devices = [];
        this.deviceServers = [];
        this.loadUsers();
    }

    onAddDeviceTemplate() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(UserConfigDialogComponent, {
                data: {
                    template: this.deviceTemplate,
                    user: this.selectedUser,
                },
                width: '65%',
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    onEditDeviceTemplate() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(UserConfigDialogComponent, {
                data: {
                    template: this.deviceTemplate,
                    user: this.selectedUser,
                },
                width: '65%',
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    private loadSelectedUserAdjacencies() {
        this.loadingUserAdjacencies = true;
        this.dataService
            .getUserAdjacencyTemplates(this.selectedUser.id)
            .pipe(
                finalize(() => {
                    this.loadingUserAdjacencies = false;
                }),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.userAdjacencies = result;
                },

                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
    }

    onDeleteUserAdjacencyClick(row: AdjacencyTemplateModel) {
        this.translateService.stream('user-list.confirmRemoveAdjacencyTemplate')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText,
                        this.dataService.deleteAdjacencyTemplate(row.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((result) => {
                if (result) {
                    this.reloadAllData();
                }
            });
    }

    onAddServerClick() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(UserAdjacencyDialogComponent, {
                width: '430px',
                data: {
                    user: this.selectedUser,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    onEditAdjacencyClick(adjacency: AdjacencyTemplateModel) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(UserAdjacencyDialogComponent, {
                width: '430px',
                data: {
                    user: this.selectedUser,
                    adjacency,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    onDeleteDeviceTemplate() {
        this.translateService.stream(['user-list.confirmRemoveDeviceTemplate', 'user-list.warningDeleteAllDevices'])
            .pipe(
                switchMap((translations) =>
                    this.appUtils.showConfirmation(
                        translations['user-list.confirmRemoveDeviceTemplate'],
                        this.dataService.deleteUserDeviceTemplate(this.selectedUser.id),
                        undefined,
                        translations['user-list.warningDeleteAllDevices']
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((result) => {
                if (result) {
                    this.reloadAllData();
                }
            });
    }

    onChangePasswordClick(selectedUser: UserModel) {
        const dialogRef = this.dialog.open(UserPasswordDialogComponent, {
            width: '400px',
            data: {
                userId: selectedUser.id,
            },
        });
        dialogRef.afterClosed().subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    loadSelectedUserTotpSecret() {
        this.loadingUserTotpSecret = true;
        this.dataService
            .getTotpSecret(this.selectedUser.id)
            .pipe(finalize(() => (this.loadingUserTotpSecret = false)))
            .subscribe((res: TotpSecretModel) => {
                this.selectedUser.totpSecret = res.totpKey;
            });
    }

    onCreateUserTotpSecretClick() {
        this.loadingUserTotpSecret = true;
        this.dataService
            .createTotpSecret(this.selectedUser.id)
            .pipe(finalize(() => (this.loadingUserTotpSecret = false)))
            .subscribe((res: TotpSecretModel) => {
                this.selectedUser.totpSecret = res.totpKey;
                this.reloadAllData();
            });
    }

    onPatchUserTotpSecretClick() {
        this.translateService.stream('user-list.confirmRecreateUserTOTP')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText,
                        this.dataService.patchTotpSecret(this.selectedUser.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((result) => {
                if (result) {
                    this.reloadAllData();
                }
            }, (err) => {
                this.appUtils.handleHttpError(err);
            });
    }

    onToggleTotpCharsShowClick() {
        this.showTotpSecretChars = !this.showTotpSecretChars;
    }

    onCopyTotpClick(secret: string) {
        this.clipboard.copy(secret);
    }

    // Device functionality methods
    onCreateDevice() {
        if (!this.deviceTemplate?.id) {
            this.appUtils.showErrorSnackBar(
                this.translateService.instant('user-list.messageNoDeviceTemplate')
            );
            return;
        }

        const dialogAfterClose: Observable<any> = this.dialog
            .open(DeviceDialogComponent, {
                data: {
                    deviceTemplateId: this.deviceTemplate.id
                },
                width: '750px'
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    private loadSelectedUserDevices() {
        if (!this.deviceTemplate?.id) {
            return;
        }
        this.loadingDevices = true;
        this.dataService
            .getDevices(this.deviceTemplate.id)
            .pipe(
                finalize(() => this.loadingDevices = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (devices) => {
                    this.devices = devices;
                    this.loadSelectedDeviceServers();
                },
                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
    }

    onSyncDevices() {
        if (!this.deviceTemplate?.id) {
            return;
        }
        this.loadingDevices = true;
        this.dataService
            .syncDevices(this.deviceTemplate.id)
            .pipe(
                finalize(() => this.loadingDevices = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.dialog.open(SyncResultDialogComponent, {
                        data: result,
                        width: '400px'
                    });
                    this.reloadAllData();
                },
                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
    }

    onViewDevice(device: any) {
        this.dialog.open(DeviceInfoDialogComponent, {
            data: {
                device: device
            },
            width: '750px'
        });
    }

    onEditDevice(device: any) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(DeviceDialogComponent, {
                data: {
                    deviceTemplateId: this.deviceTemplate.id,
                    device: device
                },
                width: '750px'
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    onDeleteDevice(device: any) {
        this.translateService.stream('user-list.confirmRemoveDevice')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText + (device.id) + '?',
                        this.dataService.deleteDevice(device.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(result => {
                if (result) {
                    this.reloadAllData();
                }
            });
    }

    onAddDeviceServer() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(DeviceServerDialogComponent, {
                data: {
                    devices: this.devices,
                    userId: this.selectedUser.id
                },
                width: '500px'
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    private loadSelectedDeviceServers() {
        if (!this.selectedUser?.id) {
            this.deviceServers = [];
            return;
        }

        this.loadingDeviceServers = true;
        this.dataService
            .getUserDeviceServerAdjacencies(this.selectedUser.id)
            .pipe(
                finalize(() => this.loadingDeviceServers = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (adjacencies) => {
                    this.deviceServers = adjacencies;
                },
                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
    }

    onSyncDeviceServers() {
        if (!this.selectedUser?.id) {
            return;
        }

        this.loadingDeviceServers = true;
        this.dataService
            .syncDeviceServers(this.selectedUser.id)
            .pipe(
                finalize(() => this.loadingDeviceServers = false),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.dialog.open(SyncResultDialogComponent, {
                        data: result,
                        width: '400px'
                    });
                    this.reloadAllData();
                },
                (err) => {
                    this.appUtils.handleHttpError(err);
                }
            );
    }

    onEditDeviceServer(deviceServer: any) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(DeviceServerDialogComponent, {
                data: {
                    devices: this.devices,
                    adjacency: deviceServer,
                    userId: this.selectedUser.id
                },
                width: '500px'
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.reloadAllData();
            }
        });
    }

    onDeleteDeviceServer(deviceServer: any) {
        this.translateService.stream('user-list.confirmRemoveDeviceServer')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText,
                        this.dataService.deleteDeviceServerAdjacency(deviceServer.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(result => {
                if (result) {
                    this.reloadAllData();
                }
            });
    }

    onTabChange(index: number): void {
        const tabNames = ['devices', 'templates'];
        const tabName = tabNames[index];

        if (tabName) {
            this.router.navigate([], {
                relativeTo: this.route,
                queryParams: { tab: tabName },
                queryParamsHandling: 'merge'
            });
        }
    }
}
