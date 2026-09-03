import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { LDAPTemplateDetailComponent } from './ldap-templates/ldaptemplate-detail/ldaptemplate-detail.component';
import { LDAPConfigsRoutingModule } from './ldap-routing.module';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { AppAgGridModule } from '../../shared/components/ag-grid/app-ag-grid.module';
import { MatIconModule } from '@angular/material/icon';
import { MatButtonModule } from '@angular/material/button';
import { MatTooltipModule } from '@angular/material/tooltip';
import { ReactiveFormsModule } from '@angular/forms';
import { MatChipsModule } from '@angular/material/chips';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelectModule } from '@angular/material/select';
import { MatAutocompleteModule } from '@angular/material/autocomplete';
import { MatStepperModule } from '@angular/material/stepper';
import { MatInputModule } from '@angular/material/input';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { LDAPConfigDialogComponent } from './ldap-configurations/ldapconfig-dialog/ldapconfig-dialog.component';
import { LDAPTemplateAddDialogComponent } from './ldap-templates/ldaptemplate-add-dialog/ldaptemplate-add-dialog.component';
import { LDAPTemplateEditDialogComponent } from './ldap-templates/ldaptemplate-edit-dialog/ldaptemplate-edit-dialog.component';
import { LDAPTemplateInterfaceDialogComponent } from './ldap-templates/ldaptemplate-interface-dialog/ldaptemplate-interface-dialog.component';
import { TranslateModule } from '@ngx-translate/core';
import { LDAPTemplateServerDialogComponent } from './ldap-templates/ldaptemplate-server-dialog/ldaptemplate-server-dialog.component';
import { MatTabsModule } from '@angular/material/tabs';
import { LdapConfigurationsComponent } from './ldap-configurations/ldap-configurations.component';
import { LdapSynchronizationComponent } from './ldap-synchronization/ldap-synchronization.component';
import { LdapTemplatesComponent } from './ldap-templates/ldap-templates.component';
import { LdapComponent } from './ldap.component';
import { LDAPErrorsComponent } from './ldap-synchronization/ldap-errors/ldap-errors.component';
import { SharedModule } from '../../shared/shared.module';

@NgModule({
    declarations: [
        LDAPTemplateDetailComponent,
        LDAPConfigDialogComponent,
        LDAPTemplateAddDialogComponent,
        LDAPTemplateEditDialogComponent,
        LDAPTemplateInterfaceDialogComponent,
        LDAPTemplateServerDialogComponent,
        LdapConfigurationsComponent,
        LdapSynchronizationComponent,
        LdapTemplatesComponent,
        LdapComponent,
        LDAPErrorsComponent,
    ],
    imports: [
        CommonModule,
        LDAPConfigsRoutingModule,
        MatProgressBarModule,
        AppAgGridModule,
        MatIconModule,
        MatButtonModule,
        MatTooltipModule,
        ReactiveFormsModule,
        MatChipsModule,
        MatProgressSpinnerModule,
        MatFormFieldModule,
        MatSelectModule,
        MatAutocompleteModule,
        MatStepperModule,
        MatInputModule,
        MatCheckboxModule,
        TranslateModule.forChild(),
        MatTabsModule,
        SharedModule,
    ],
})
export class LDAPModule { }
