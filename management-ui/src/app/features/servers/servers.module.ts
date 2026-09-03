import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ServersRoutingModule } from './servers-routing.module';
import { ServersListComponent } from './servers-list/servers-list.component';
import { MatIconModule } from '@angular/material/icon';
import { MatPaginatorModule } from '@angular/material/paginator';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { AppAgGridModule } from '../../shared/components/ag-grid/app-ag-grid.module';
import { MatButtonModule } from '@angular/material/button';
import { AddServerDialogComponent } from './servers-list/add-server-dialog/add-server-dialog.component';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { ReactiveFormsModule } from '@angular/forms';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatSelectModule } from '@angular/material/select';
import { MatDialogModule } from '@angular/material/dialog';
import { CommonFormDialogComponent } from '../../shared/dialogs/common-form-dialog/common-form-dialog.component';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ServerConfigDialogComponent } from './servers-list/server-config-dialog/server-config-dialog.component';
import { MatChipsModule } from '@angular/material/chips';
import { MatAutocompleteModule } from '@angular/material/autocomplete';
import { ServerAdjacencyDialogComponent } from './servers-list/server-adjacency-dialog/server-adjacency-dialog.component';
import { MatStepperModule } from '@angular/material/stepper';
import { EditServerDialogComponent } from './servers-list/edit-server-dialog/edit-server-dialog.component';
import { TranslateModule } from "@ngx-translate/core";
import { SharedModule } from '../../shared/shared.module';


@NgModule({
    declarations: [ServersListComponent, AddServerDialogComponent, CommonFormDialogComponent, ServerConfigDialogComponent, ServerAdjacencyDialogComponent, EditServerDialogComponent],
    imports: [
        CommonModule,
        ServersRoutingModule,
        MatIconModule,
        MatPaginatorModule,
        MatProgressBarModule,
        AppAgGridModule,
        MatButtonModule,
        MatFormFieldModule,
        MatInputModule,
        ReactiveFormsModule,
        MatTooltipModule,
        MatSelectModule,
        MatDialogModule,
        MatProgressSpinnerModule,
        MatChipsModule,
        MatAutocompleteModule,
        MatStepperModule,
        SharedModule,
        TranslateModule.forChild(),
    ]
})
export class ServersModule {
}
