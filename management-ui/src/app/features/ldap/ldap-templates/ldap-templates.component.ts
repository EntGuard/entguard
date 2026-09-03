import { Component, OnDestroy, OnInit, ViewChild } from '@angular/core';
import { MatDialog } from '@angular/material/dialog';
import { TranslateService } from '@ngx-translate/core';
import { ColDef } from 'ag-grid-community';
import { Observable, Subject } from 'rxjs';
import { finalize, switchMap, takeUntil } from 'rxjs/operators';
import { AppUtilsService } from 'src/app/core/app-utils.service';
import { LDAPService } from 'src/app/features/ldap/ldap.service';
import { LdapTemplateModel } from 'src/app/features/ldap/models/ldap-template-model';
import { AppAgGridComponent } from 'src/app/shared/components/ag-grid/app-ag-grid.component';
import { LDAPTemplateAddDialogComponent } from './ldaptemplate-add-dialog/ldaptemplate-add-dialog.component';
import { LDAPTemplateEditDialogComponent } from './ldaptemplate-edit-dialog/ldaptemplate-edit-dialog.component';
import { Router } from '@angular/router';

@Component({
    selector: 'app-ldap-templates',
    templateUrl: './ldap-templates.component.html',
    styleUrls: ['./ldap-templates.component.scss'],
    standalone: false,
})
export class LdapTemplatesComponent implements OnInit, OnDestroy {

    @ViewChild('templatesGrid') templatesGrid: AppAgGridComponent;
    private componentDestroyed: Subject<void> = new Subject<void>();

    loading = true;
    error: any;
    templates: LdapTemplateModel[];
    templateCols: ColDef[] = [
        {
            colId: 'name',
            field: 'name',
            headerName: 'ldaptemplate-dialog.labelName'
        },
        {
            colId: 'filter',
            field: 'filter',
            headerName: 'ldaptemplate-dialog.labelFilter'
        },
        {
            colId: 'isAdmin',
            field: 'isAdmin',
            headerName: 'ldaptemplate-dialog.checkAdmin',
            cellDataType: 'text'
        },
        {
            colId: 'mfaType',
            field: 'mfaType',
            headerName: 'ldaptemplate-dialog.labelMfaType'
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
        private dataService: LDAPService,
        private appUtils: AppUtilsService,
        private translateService: TranslateService,
        private router: Router,
    ) { }

    ngOnInit(): void {
        this.translateHeaderNames();
        this.loadTemplates();
    }

    private translateHeaderNames(): void {
        this.translateService.stream('ldaptemplate-dialog').pipe(takeUntil(this.componentDestroyed)).subscribe(() => {
            this.templateCols.forEach(templateCols => {
                if (templateCols.headerName) {
                    templateCols.headerName = this.translateService.instant(templateCols.headerName);
                }
            });
        });
    }

    loadTemplates() {
        this.dataService
            .getTemplates()
            .pipe(
                finalize(() => (this.loading = false)),
                takeUntil(this.componentDestroyed)
            )
            .subscribe(
                (templates: LdapTemplateModel[]) => {
                    this.templates = templates;
                },
                (err) => (this.error = err)
            );
    }

    onAddTemplateClick() {
        const dialogRef = this.dialog.open(LDAPTemplateAddDialogComponent, { disableClose: true, data: {} });

        dialogRef.componentInstance.onAddTemplate.subscribe((res: LdapTemplateModel[]) => {
            this.templates = res;
        });

        dialogRef.afterClosed().subscribe((response) => {
            if (response && response.configureTemplateId) {
                this.router.navigate(['/ldap', 'templates', response.configureTemplateId, 'servers']);
            }
        });
    }

    onEditTemplateClick(selectedTemplate: LdapTemplateModel) {
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
                this.templatesGrid.refreshView();
            }
        });
    }

    onDeleteTemplateClick(selectedTemplate: LdapTemplateModel) {
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
                    this.templates = result;
                }
            });
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }
}
