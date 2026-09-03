import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';
import { AddressPoolsComponent } from './address-pools.component';

const routes: Routes = [
    {
        path: '',
        component: AddressPoolsComponent
    }
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule]
})
export class AddressPoolsRoutingModule { }
