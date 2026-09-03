import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { UsersListComponent } from './users-list/users-list/users-list.component';
import { UsersRoutingModule } from './users-routing.module';
import { UserAdjacencyDialogComponent } from './users-list/users-list/user-adjacency-dialog/user-adjacency-dialog.component';
import { UserConfigDialogComponent } from './users-list/users-list/user-config-dialog/user-config-dialog.component';
import { UserDialogComponent } from './users-list/users-list/user-dialog/user-dialog.component';
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
import { MatInputModule } from '@angular/material/input';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatAutocompleteModule } from '@angular/material/autocomplete';
import { UserPasswordDialogComponent } from './users-list/users-list/user-password-dialog/user-password-dialog.component';
import { DeviceDialogComponent } from './users-list/users-list/device-dialog/device-dialog.component';
import { DeviceServerDialogComponent } from './users-list/users-list/device-server-dialog/device-server-dialog.component';
import { DeviceInfoDialogComponent } from './users-list/users-list/device-info-dialog/device-info-dialog.component';
import { TranslateModule } from '@ngx-translate/core';
import { MatTabsModule } from '@angular/material/tabs';
import { SharedModule } from '../../shared/shared.module';

@NgModule({
    declarations: [
        UsersListComponent,
        UserAdjacencyDialogComponent,
        UserConfigDialogComponent,
        UserDialogComponent,
        UserPasswordDialogComponent,
        DeviceDialogComponent,
        DeviceServerDialogComponent,
        DeviceInfoDialogComponent,
    ],
    imports: [
        CommonModule,
        UsersRoutingModule,
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
        MatInputModule,
        MatCheckboxModule,
        MatAutocompleteModule,
        SharedModule,
        TranslateModule.forChild(),
        MatTabsModule,
    ],
})
export class UsersModule { }
