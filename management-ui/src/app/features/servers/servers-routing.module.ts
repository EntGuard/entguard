import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';
import { ServersListComponent } from './servers-list/servers-list.component';

const routes: Routes = [
    {
        path: '',
        component: ServersListComponent,
    },
    {
        path: ':server',
        component: ServersListComponent
    }
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule]
})
export class ServersRoutingModule { }
