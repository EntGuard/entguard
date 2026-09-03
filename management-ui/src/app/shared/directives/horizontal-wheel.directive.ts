import { Directive, HostListener } from '@angular/core';

/**
 * Convert vertical wheel movement into horizontal scroll on overflowed elements.
 * Usage: add appHorizontalWheel to any scrollable container.
 */
@Directive({
    selector: '[appHorizontalWheel]',
    standalone: true,
})
export class HorizontalWheelDirective {

    @HostListener('wheel', ['$event'])
    onWheel(event: WheelEvent): void {
        const el = event.currentTarget as HTMLElement | null;
        if (!el) { return; }

        const canScrollX = el.scrollWidth > el.clientWidth;
        if (!canScrollX) { return; }

        // If vertical movement dominates, translate it into horizontal scroll
        const useDeltaY = Math.abs(event.deltaY) >= Math.abs(event.deltaX);
        if (useDeltaY) {
            el.scrollLeft += event.deltaY;
            event.preventDefault();
            event.stopPropagation();
        }
    }
}
