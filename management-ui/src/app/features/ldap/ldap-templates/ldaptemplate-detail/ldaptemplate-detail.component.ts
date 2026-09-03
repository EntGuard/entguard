import { Component, OnDestroy, OnInit } from '@angular/core';
import { ColDef } from 'ag-grid-community';
import { LdapTemplateModel } from '../../models/ldap-template-model';
import { LdapInterfaceModel } from '../../models/ldap-template-interface-model';
import { LdapServerModel } from '../../models/ldap-template-server-model';
import { Observable, Subject } from 'rxjs';
import { LDAPService } from '../../ldap.service';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { AppUtilsService } from '../../../../core/app-utils.service';
import { MatDialog } from '@angular/material/dialog';
import { ActivatedRoute, Params, Router } from '@angular/router';
import { LDAPTemplateInterfaceDialogComponent } from '../ldaptemplate-interface-dialog/ldaptemplate-interface-dialog.component';
import { LDAPTemplateServerDialogComponent } from '../ldaptemplate-server-dialog/ldaptemplate-server-dialog.component';
import { LDAPTemplateEditDialogComponent } from '../ldaptemplate-edit-dialog/ldaptemplate-edit-dialog.component';
import { TranslateService } from '@ngx-translate/core';
import { Location } from '@angular/common';

@Component({
    selector: 'app-ldaptemplate-detail',
    templateUrl: './ldaptemplate-detail.component.html',
    styleUrls: ['./ldaptemplate-detail.component.scss'],
    standalone: false,
})
export class LDAPTemplateDetailComponent implements OnInit, OnDestroy {

    private componentDestroyed: Subject<void> = new Subject<void>();
    loadingInterface = true;
    loadingServers = true;
    loadingTemplate = true;

    private templateId: number;

    error: any;
    serversError: any;
    interfaceError: any;
    selectedTemplate: LdapTemplateModel;
    iface: LdapInterfaceModel | null;
    servers: LdapServerModel[];
    serversCols: ColDef[] = [
        {
            colId: 'server',
            field: 'name',
            headerName: 'ldapconfigs-template-detail-servers.labelName',
            filter: true,
        },
        {
            colId: 'allowedIps',
            field: 'allowedIps',
            headerName: 'ldapconfigs-template-detail-servers.allowedAddresses',
            filter: true,
        },
        {
            colId: 'usePresharedKey',
            field: 'usePresharedKey',
            headerName: 'ldapconfigs-template-detail-servers.usePresharedKey',
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
        private dataService: LDAPService,
        private dialog: MatDialog,
        private route: ActivatedRoute,
        private appUtils: AppUtilsService,
        private translateService: TranslateService,
        private router: Router,
        private location: Location
    ) { }

    ngOnInit(): void {
        this.translateHeaderNames();
        this.loadSelectedTemplate();
    }

    private translateHeaderNames(): void {
        this.translateService.stream('ldapconfigs-template-detail-servers').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            this.serversCols.forEach(col => {
                if (col.headerName) {
                    col.headerName = this.translateService.instant(col.headerName);
                }
            });
        });
    }

    loadSelectedTemplate() {
        this.route.params.subscribe(
            (params: Params) => {
                if (params['id']) {
                    this.templateId = +params['id'];
                    this.dataService
                        .getTemplates()
                        .pipe(
                            finalize(() => (this.loadingTemplate = false)),
                            takeUntil(this.componentDestroyed)
                        )
                        .subscribe((templates: LdapTemplateModel[]) => {
                            const foundTemplate = templates.find(t => t.id === this.templateId);
                            if (foundTemplate) {
                                this.setSelectedTemplate(foundTemplate);
                            } else {
                                this.error = 'Template not found';
                                this.loadingInterface = false;
                                this.loadingServers = false;
                            }
                        });
                }
            },
            (err) => (this.error = err)
        );
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    onEditSelectedTemplateClick(selectedTemplate: LdapTemplateModel) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPTemplateEditDialogComponent, {
                data: {
                    template: selectedTemplate,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                selectedTemplate.setDataFromModel(
                    response.find(
                        (template) => template.id === selectedTemplate.id
                    )
                );
                this.location.replaceState(
                    '/ldap/templates/' + selectedTemplate.id + '/servers'
                );
            }
        });
    }

    onDeleteSelectedTemplateClick(selectedTemplate: LdapTemplateModel) {
        this.translateService.stream('ldapconfigs-list.removeLdapTemplate')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText + selectedTemplate.name + '?',
                        this.dataService.deleteTemplate(selectedTemplate.id),
                        this.dataService.getTemplates()
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((result) => {
                if (result) {
                    this.router.navigate(['/ldap/templates']);
                }
            });
    }

    onAddInterface() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPTemplateInterfaceDialogComponent, {
                data: {
                    template: this.selectedTemplate,
                    iface: this.iface,
                },
                width: '65%',
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.iface = response;
            }
        });
    }

    onEditInterface() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPTemplateInterfaceDialogComponent, {
                data: {
                    template: this.selectedTemplate,
                    iface: this.iface,
                },
                width: '65%',
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.iface = response;
            }
        });
    }

    onAddServerClick() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPTemplateServerDialogComponent, {
                data: {
                    template: this.selectedTemplate,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.servers = response;
            }
        });
    }

    onEditServerClick(server: LdapServerModel) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPTemplateServerDialogComponent, {
                data: {
                    template: this.selectedTemplate,
                    server,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.servers = response;
            }
        });
    }

    onDeleteServerClick(row) {
        this.translateService.stream('ldapconfigs-list.removeLdapServer')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText,
                        this.dataService.deleteTemplateServer(this.selectedTemplate.id, row.id),
                        this.dataService.getTemplateServers(this.selectedTemplate.id)
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((result) => {
                if (result) {
                    this.servers = result;
                }
            });
    }

    private loadSelectedTemplateServers(): void {
        this.dataService
            .getTemplateServers(this.templateId)
            .pipe(
                finalize(() => {
                    this.loadingServers = false;
                }),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (result) => {
                    this.servers = result;
                },
                (err) => (this.serversError = err)
            );
    }

    private setSelectedTemplate(template: LdapTemplateModel) {
        this.templateId = template.id;
        this.selectedTemplate = template;
        this.iface = new LdapInterfaceModel({
            iface_name: template.interfaceName,
            iface_addrs: template.addressPools,
            listen_port: template.listenPort,
            dns: template.dns,
            mtu: template.mtu
        });
        this.loadingInterface = false;
        this.loadSelectedTemplateServers();
    }
}
