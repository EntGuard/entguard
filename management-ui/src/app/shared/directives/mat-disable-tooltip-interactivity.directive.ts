import { Directive, Input, OnChanges, SimpleChanges, Host, Self, Optional } from '@angular/core';
import { MatTooltip } from '@angular/material/tooltip';

@Directive({
    selector: '[matDisableTooltipInteractivity]',
    standalone: true
})
export class MatDisableTooltipInteractivityDirective implements OnChanges {
    @Input() matDisableTooltipInteractivity: boolean | string = true;

    constructor(@Host() @Self() @Optional() private matTooltip: MatTooltip) { }

    ngOnChanges(changes: SimpleChanges): void {
        if (changes['matDisableTooltipInteractivity']) {
            this.updateInteractivity();
        }
    }

    private updateInteractivity(): void {
        if (!this.matTooltip) {
            return;
        }

        const interactableClass = 'mat-tooltip-interactable';
        const isDisabled = this.matDisableTooltipInteractivity === true || this.matDisableTooltipInteractivity === 'true';

        if (!isDisabled) {
            // Enable interactivity
            let currentClass = this.matTooltip.tooltipClass || '';
            if (Array.isArray(currentClass)) {
                if (!currentClass.includes(interactableClass)) {
                    this.matTooltip.tooltipClass = [...currentClass, interactableClass];
                }
            } else if (typeof currentClass === 'string') {
                if (!currentClass.includes(interactableClass)) {
                    this.matTooltip.tooltipClass = `${currentClass} ${interactableClass}`.trim();
                }
            }
        } else {
            // Disable interactivity (remove the class)
            let currentClass = this.matTooltip.tooltipClass || '';
            if (Array.isArray(currentClass)) {
                this.matTooltip.tooltipClass = currentClass.filter(c => c !== interactableClass);
            } else if (typeof currentClass === 'string') {
                this.matTooltip.tooltipClass = currentClass.replace(interactableClass, '').trim();
            }
        }
    }
}
