import { Component, ChangeDetectionStrategy } from '@angular/core';

@Component({
    selector: 'app-premium-feature',
    templateUrl: './premium-feature.component.html',
    styleUrls: ['./premium-feature.component.scss'],
    changeDetection: ChangeDetectionStrategy.Eager,
    standalone: false,
})
export class PremiumFeatureComponent {
    constructor() { }
}
