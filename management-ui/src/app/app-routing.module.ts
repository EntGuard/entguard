import { NgModule } from '@angular/core';
import { Routes, RouterModule } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { AuthGuard } from './core/guards/auth.guard';
import { LdapFeatureGuard } from './core/guards/ldap-feature-guard.service';
import { PremiumFeatureComponent } from './features/unsupported-feature/premium-feature.component';

const routes: Routes = [
    {
        path: '',
        pathMatch: 'full',
        redirectTo: '/login',
    },
    {
        path: 'login',
        component: LoginComponent,
    },
    {
        path: 'premium',
        component: PremiumFeatureComponent,
    },
    {
        path: 'servers',
        loadChildren: () =>
            import('./features/servers/servers.module').then(
                (mod) => mod.ServersModule
            ),
        canActivate: [AuthGuard],
    },
    {
        path: 'users',
        loadChildren: () =>
            import('./features/users/users.module').then(
                (mod) => mod.UsersModule
            ),
        canActivate: [AuthGuard],
    },
    {
        path: 'address-pools',
        loadChildren: () =>
            import('./features/address-pools/address-pools.module').then(
                (mod) => mod.AddressPoolsModule
            ),
        canActivate: [AuthGuard],
    },
    {
        path: 'ldap',
        loadChildren: () =>
            import('./features/ldap/ldap.module').then((mod) => mod.LDAPModule),
        canActivate: [LdapFeatureGuard],
    },
    {
        path: 'mfa-certificates',
        loadChildren: () =>
            import('./features/mfa-certificates/mfa-certificates.module').then(
                (mod) => mod.MfaCertificatesModule
            ),
        canActivate: [AuthGuard],
    },
];

@NgModule({
    imports: [RouterModule.forRoot(routes)],
    exports: [RouterModule],
})
export class AppRoutingModule { }
