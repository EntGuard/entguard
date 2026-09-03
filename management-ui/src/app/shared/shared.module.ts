import { NgModule, ModuleWithProviders } from '@angular/core';
import { CommonModule } from '@angular/common';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatButtonModule } from '@angular/material/button';
import { MatDialogModule } from '@angular/material/dialog';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSelectModule } from '@angular/material/select';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { ReactiveFormsModule } from '@angular/forms';
import { AppAgGridModule } from '../shared/components/ag-grid/app-ag-grid.module';
import { TranslateModule } from '@ngx-translate/core';
import { HorizontalWheelDirective } from './directives/horizontal-wheel.directive';
import { MatDisableTooltipInteractivityDirective } from './directives/mat-disable-tooltip-interactivity.directive';
import { MatFormFieldModule } from '@angular/material/form-field';
import { AddressPoolMultiSelectComponent } from './components/address-pool-multi-select/address-pool-multi-select.component';
import { MatPseudoCheckboxModule } from '@angular/material/core';
import { SyncResultDialogComponent } from './dialogs/sync-result-dialog/sync-result-dialog.component';

@NgModule({
    declarations: [
        AddressPoolMultiSelectComponent,
        SyncResultDialogComponent
    ],
    imports: [
        CommonModule,
        MatIconModule,
        MatInputModule,
        MatButtonModule,
        MatDialogModule,
        MatProgressBarModule,
        MatProgressSpinnerModule,
        MatSelectModule,
        MatFormFieldModule,
        MatTooltipModule,
        MatCheckboxModule,
        MatPseudoCheckboxModule,
        ReactiveFormsModule,
        AppAgGridModule,
        HorizontalWheelDirective,
        MatDisableTooltipInteractivityDirective,
        TranslateModule
    ],
    exports: [
        CommonModule,
        MatIconModule,
        MatInputModule,
        MatButtonModule,
        MatDialogModule,
        MatProgressBarModule,
        MatProgressSpinnerModule,
        MatSelectModule,
        MatFormFieldModule,
        MatTooltipModule,
        MatCheckboxModule,
        MatPseudoCheckboxModule,
        ReactiveFormsModule,
        AppAgGridModule,
        TranslateModule,
        HorizontalWheelDirective,
        MatDisableTooltipInteractivityDirective,
        AddressPoolMultiSelectComponent,
        SyncResultDialogComponent
    ]
})
export class SharedModule {
    public static forRoot(): ModuleWithProviders<SharedModule> {
        return {
            ngModule: SharedModule,
            providers: []
        };
    }
}

