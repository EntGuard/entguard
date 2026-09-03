import { Component, ElementRef, EventEmitter, Input, OnDestroy, Output, ViewChild } from '@angular/core';
import { FormControl } from '@angular/forms';
import { MatOptionSelectionChange, MatPseudoCheckboxState } from '@angular/material/core';
import { BehaviorSubject, combineLatest, Observable, Subscription } from 'rxjs';
import { map, startWith } from 'rxjs/operators';
import { AddressPoolModel } from '../../models/address-pool.model';

interface AddressPoolOptionViewModel {
    pool: AddressPoolModel;
    visible: boolean;
}

interface AddressPoolSelectViewModel {
    filteredPools: AddressPoolModel[];
    options: AddressPoolOptionViewModel[];
}

@Component({
    selector: 'app-address-pool-multi-select',
    templateUrl: './address-pool-multi-select.component.html',
    styleUrls: ['./address-pool-multi-select.component.scss'],
    standalone: false,
})
export class AddressPoolMultiSelectComponent implements OnDestroy {
    private readonly fallbackControl = new FormControl<AddressPoolModel[]>([]);
    private _control: FormControl<AddressPoolModel[]> = this.fallbackControl;
    private poolsSubject = new BehaviorSubject<AddressPoolModel[]>([]);
    private controlSub?: Subscription;

    @Input() loading = false;
    @Input() disabled = false;
    @Input() required = false;
    @Output() selectionChange = new EventEmitter<AddressPoolModel[]>();

    @Input()
    set pools(value: AddressPoolModel[]) {
        this.poolsSubject.next(value ?? []);
    }
    get pools(): AddressPoolModel[] {
        return this.poolsSubject.value;
    }

    @Input()
    set control(value: FormControl<AddressPoolModel[]> | undefined) {
        const nextControl = value ?? this.fallbackControl;
        if (nextControl === this._control) {
            return;
        }
        this._control = nextControl;
        this.subscribeToControl();
    }
    get control(): FormControl<AddressPoolModel[]> {
        return this._control;
    }

    @ViewChild('searchInput') searchInput: ElementRef<HTMLInputElement>;

    searchControl = new FormControl('');
    vm$: Observable<AddressPoolSelectViewModel> = combineLatest([
        this.poolsSubject.asObservable(),
        this.searchControl.valueChanges.pipe(startWith('')),
    ]).pipe(
        map(([pools, searchValue]) => {
            const filteredPools = this.filterPools(
                pools,
                (searchValue || '').toString()
            );
            const visibleIds = new Set(filteredPools.map((pool) => pool.id));
            const options = pools.map((pool) => ({
                pool,
                visible: visibleIds.has(pool.id),
            }));

            return {
                filteredPools,
                options,
            };
        })
    );

    constructor() {
        this.subscribeToControl();
    }

    ngOnDestroy(): void {
        this.controlSub?.unsubscribe();
        this.poolsSubject.complete();
    }

    trackById(_: number, pool: AddressPoolModel): number {
        return pool.id;
    }

    trackOptionById(_: number, option: AddressPoolOptionViewModel): number {
        return option.pool.id;
    }

    get selectedCount(): number {
        return this.control?.value?.length || 0;
    }

    get selectedLabel(): string {
        const current = this.control?.value || [];
        if (current.length === 0) {
            return '';
        }
        if (current.length === 1) {
            return current[0].name;
        }
        return `${current[0].name} +${current.length - 1}`;
    }

    removePool(pool: AddressPoolModel, event: MouseEvent): void {
        event.preventDefault();
        event.stopPropagation();

        if (this.disabled || this.loading) {
            return;
        }

        const current = this.control.value || [];
        const next = current.filter((p: AddressPoolModel) => p.id !== pool.id);
        this.control.setValue(next);
        this.control.markAsDirty();
        this.control.markAsTouched();
    }

    get showError(): boolean {
        return (
            !!this.control &&
            this.control.invalid &&
            (this.control.dirty || this.control.touched)
        );
    }

    onPanelOpened(opened: boolean): void {
        if (opened) {
            setTimeout(() => this.searchInput?.nativeElement?.focus(), 75);
            return;
        }
        this.control?.markAsTouched();
        this.searchControl.setValue('');
    }

    clearSearch(event: MouseEvent): void {
        event.preventDefault();
        event.stopPropagation();
        this.searchControl.setValue('');
    }

    comparePools = (a: AddressPoolModel, b: AddressPoolModel): boolean => {
        if (a === b) {
            return true;
        }
        if (!a || !b) {
            return false;
        }
        return a.id === b.id;
    };

    areAllFilteredSelected(pools: AddressPoolModel[]): boolean {
        if (!pools.length) {
            return false;
        }
        const selectedIds = this.getSelectedIds();
        return pools.every((pool) => selectedIds.has(pool.id));
    }

    getSelectAllState(pools: AddressPoolModel[]): MatPseudoCheckboxState {
        if (!pools.length) {
            return 'unchecked';
        }

        const selectedIds = this.getSelectedIds();
        const selectedCount = pools.reduce(
            (count, pool) => (selectedIds.has(pool.id) ? count + 1 : count),
            0
        );

        if (selectedCount === pools.length) {
            return 'checked';
        }

        if (selectedCount === 0) {
            return 'unchecked';
        }

        return 'unchecked';
    }

    onSelectAllClick(pools: AddressPoolModel[], event: MouseEvent): void {
        // Don't toggle if clicking on the input or clear button
        const target = event.target as HTMLElement;
        if (target.tagName === 'INPUT' || target.classList.contains('clear-btn')) {
            return;
        }
        this.toggleFilteredSelection(pools);
    }

    private toggleFilteredSelection(pools: AddressPoolModel[]): void {
        if (!pools.length) {
            return;
        }

        const current = this.control.value || [];
        const selectedIds = this.getSelectedIds();
        let next: AddressPoolModel[];

        if (this.areAllFilteredSelected(pools)) {
            const filteredIds = new Set(pools.map((pool) => pool.id));
            next = current.filter((pool) => !filteredIds.has(pool.id));
        } else {
            const additions = pools.filter((pool) => !selectedIds.has(pool.id));
            next = [...current, ...additions];
        }

        this.control.setValue(next);
        this.control.markAsDirty();
        this.control.markAsTouched();
    }

    private getSelectedIds(): Set<number> {
        return new Set((this.control.value || []).map((pool) => pool.id));
    }

    private subscribeToControl(): void {
        this.controlSub?.unsubscribe();
        this.controlSub = this.control.valueChanges
            .pipe(startWith(this.control.value || []))
            .subscribe((value) => this.selectionChange.emit(value || []));
    }

    private filterPools(
        pools: AddressPoolModel[],
        searchValue: string
    ): AddressPoolModel[] {
        const normalized = searchValue.trim().toLowerCase();
        if (!normalized) {
            return pools;
        }
        return pools.filter((pool) => {
            const tokens = [pool.name, pool.addressRange, pool.description]
                .filter(Boolean)
                .map((token) => token!.toLowerCase());
            return tokens.some((token) => token.includes(normalized));
        });
    }
}
