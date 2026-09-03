import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';
import { MfaCertificatesComponent } from './mfa-certificates.component';


const routes: Routes = [
    {
        path: '',
        component: MfaCertificatesComponent,
    },
];


@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule]
})
export class MfaCertificatesRoutingModule {}
