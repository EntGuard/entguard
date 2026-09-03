import {
    NgModule,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppAgGridComponent } from './app-ag-grid.component';
import { AgCellTemplateRendererComponent } from './ag-cell-template-renderer/ag-cell-template-renderer.component';
import { AgGridModule } from 'ag-grid-angular';
import { FormsModule } from '@angular/forms';
import { MatSelectModule } from '@angular/material/select';
import { MatButtonModule } from '@angular/material/button';
import { MatTooltipModule } from '@angular/material/tooltip';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatFormFieldModule } from '@angular/material/form-field';
import { TranslateModule } from '@ngx-translate/core';

@NgModule({
    declarations: [AppAgGridComponent, AgCellTemplateRendererComponent],
    imports: [
        CommonModule,
        AgGridModule,
        FormsModule,
        MatSelectModule,
        MatButtonModule,
        MatTooltipModule,
        MatIconModule,
        MatInputModule,
        MatCheckboxModule,
        MatFormFieldModule,
        TranslateModule.forChild(),
    ],
    exports: [AppAgGridComponent],
})
export class AppAgGridModule {
}
