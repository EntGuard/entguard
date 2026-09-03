import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';
import { UsersListComponent } from './users-list/users-list/users-list.component';

const routes: Routes = [
    {
        path: '',
        component: UsersListComponent,
    },
    {
        path: ':user',
        component: UsersListComponent,
    },
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule],
})
export class UsersRoutingModule {}
