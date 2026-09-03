import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';
import { LdapComponent } from './ldap.component';
import { LdapConfigurationsComponent } from './ldap-configurations/ldap-configurations.component';
import { LdapSynchronizationComponent } from './ldap-synchronization/ldap-synchronization.component';
import { LdapTemplatesComponent } from './ldap-templates/ldap-templates.component';
import { LDAPTemplateDetailComponent } from './ldap-templates/ldaptemplate-detail/ldaptemplate-detail.component';

const routes: Routes = [
    {
        path: '',
        component: LdapComponent,
        children: [
            {
                path: '',
                redirectTo: 'synchronization-status',
                pathMatch: 'full',
            },
            {
                path: 'synchronization-status',
                component: LdapSynchronizationComponent,
            },
            {
                path: 'configurations',
                component: LdapConfigurationsComponent,
            },
            {
                path: 'templates',
                component: LdapTemplatesComponent,
            },
            {
                path: 'templates/:id/servers',
                component: LDAPTemplateDetailComponent,
            },
            {
                path: 'templates/:id',
                component: LDAPTemplateDetailComponent,
            },
        ],
    },
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule],
})
export class LDAPConfigsRoutingModule { }
