import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AddressPoolsComponent } from './address-pools.component';
import { AddressPoolDialogComponent } from './address-pool-dialog/address-pool-dialog.component';
import { AddressPoolsRoutingModule } from './address-pools-routing.module';
import { TranslateModule } from '@ngx-translate/core';
import { SharedModule } from '../../shared/shared.module';

@NgModule({
    declarations: [
        AddressPoolsComponent,
        AddressPoolDialogComponent
    ],
    imports: [
        CommonModule,
        SharedModule,
        TranslateModule.forChild(),
        AddressPoolsRoutingModule
    ],
    exports: [
        AddressPoolsComponent
    ]
})
export class AddressPoolsModule { }
