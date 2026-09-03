import { Component, OnDestroy, OnInit } from '@angular/core';
import { MatDialog } from '@angular/material/dialog';
import { TranslateService } from '@ngx-translate/core';
import { ColDef } from 'ag-grid-community';
import { Observable, Subject } from 'rxjs';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { AppUtilsService } from 'src/app/core/app-utils.service';
import { LDAPService } from 'src/app/features/ldap/ldap.service';
import { LdapConfigModel } from 'src/app/features/ldap/models/ldap-config-model';
import { LdapTemplateModel } from 'src/app/features/ldap/models/ldap-template-model';
import { LDAPConfigDialogComponent } from './ldapconfig-dialog/ldapconfig-dialog.component';

@Component({
    selector: 'app-ldap-configurations',
    templateUrl: './ldap-configurations.component.html',
    styleUrls: ['./ldap-configurations.component.scss'],
    standalone: false,
})
export class LdapConfigurationsComponent implements OnInit, OnDestroy {
    private componentDestroyed: Subject<void> = new Subject<void>();
    templates: LdapTemplateModel[];
    loadingTemplates = true;
    loadingConfigs = true;
    configs: LdapConfigModel[];
    configCols: ColDef[] = [
        {
            colId: 'host',
            field: 'host',
            headerName: 'ldapconfig-dialog.inputLabelHost'
        },
        {
            colId: 'priority',
            field: 'priority',
            headerName: 'ldapconfig-dialog.inputLabelPriority'
        },
        {
            colId: 'template',
            field: 'template',
            valueGetter: (params) => this.getTemplateName(params.data.template),
            headerName: 'ldapconfig-dialog.labelTemplate'
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
    error: any;

    constructor(
        private dataService: LDAPService,
        private dialog: MatDialog,
        private appUtils: AppUtilsService,
        private translateService: TranslateService
    ) { }

    ngOnInit(): void {
        this.translateHeaderNames();
        this.loadConfigs();
        this.loadTemplates();
    }

    private translateHeaderNames(): void {
        this.translateService.stream('ldapconfig-dialog').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            this.configCols.forEach(col => {
                if (col.headerName) {
                    col.headerName = this.translateService.instant(col.headerName);
                }
            });
        });
    }

    private loadConfigs() {
        this.dataService
            .getConfigs()
            .pipe(
                finalize(() => (this.loadingConfigs = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (configs: LdapConfigModel[]) => {
                    this.configs = configs;
                },
                (err) => (this.error = err)
            );
    }

    private loadTemplates() {
        this.dataService
            .getTemplates()
            .pipe(
                finalize(() => (this.loadingTemplates = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (templates: LdapTemplateModel[]) => {
                    this.templates = templates;
                },
                (err) => (this.error = err)
            );
    }

    onAddConfigurationClick() {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPConfigDialogComponent, { data: {} })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.configs = response;
            }
        });
    }

    private getTemplateName(id) {
        const template = this.templates.find((t) => t.id === id);

        if (template) {
            return template.name;
        }

        return null;
    }

    onEditConfigurationClick(configuration: LdapConfigModel) {
        const dialogAfterClose: Observable<any> = this.dialog
            .open(LDAPConfigDialogComponent, {
                data: {
                    config: configuration,
                },
            })
            .afterClosed();

        dialogAfterClose.subscribe((response) => {
            if (response) {
                this.configs = response;
            }
        });
    }

    onDeleteConfigClick(row: any) {
        this.translateService.stream('ldapconfigs-list.removeLdapConfig')
            .pipe(
                switchMap((translatedText) =>
                    this.appUtils.showConfirmation(
                        translatedText,
                        this.dataService.deleteConfig(row['id']),
                        this.dataService.getConfigs()
                    )
                ),
                takeUntil(this.componentDestroyed)
            )
            .subscribe((result) => {
                if (result) {
                    this.configs = result;
                }
            });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }
}
