import {
    AfterViewInit,
    Component,
    ContentChildren,
    ElementRef,
    EventEmitter,
    Input,
    OnDestroy,
    OnInit,
    Output,
    QueryList,
    TemplateRef,
} from '@angular/core';
import {
    ColDef,
    GridApi,
    GridOptions,
    GridSizeChangedEvent,
    ITooltipParams,
    IRowNode,
    GetRowIdParams,
    RowClassParams,
} from 'ag-grid-community';
import { AgCellTemplateRendererComponent } from './ag-cell-template-renderer/ag-cell-template-renderer.component';
import { Observable, Subject, Subscription } from 'rxjs';
import { AppUtilsService } from '../../../core/app-utils.service';
import { MatSelectChange } from '@angular/material/select';

@Component({
    selector: 'app-ag-grid',
    templateUrl: './app-ag-grid.component.html',
    styleUrls: ['./app-ag-grid.component.scss'],
    standalone: false,
})
export class AppAgGridComponent implements AfterViewInit, OnInit, OnDestroy {
    @Input() tableId: string;

    @Input() entityName: string;
    @Input() entityNamePlural: string;
    /**
     * List of all column definitions, that can be displayed in table
     */
    @Input() allColumnDefs: ColDef[] = [];
    @Input() gridOptions: GridOptions;
    @Input() colsInstanceIdentifiers: { [colId: string]: string };
    @Input() colsCustomDescTooltips: { [colId: string]: string };

    @Input() visible: boolean;

    @Input() headerHeight = 50;
    @Input() rowHeight = 50;
    @Input() rowData: any = undefined;
    @Input() pagination = true;
    @Input() paginationPageSize = 25;

    @Input() rowSelection = undefined;
    @Input() suppressRowClickSelection = true;

    @Input() translationPropertyKeyPrefix = '';
    @Input() doNotUppercaseTranslationKeys = false;
    @Input() headerCheckboxSelection = true;
    @Input() multiselection = false;

    @Input() components: Object;

    @Output() rowClicked: EventEmitter<any> = new EventEmitter<any>();
    @Output() cellMouseOver: EventEmitter<any> = new EventEmitter<any>();
    @Output() cellMouseOut: EventEmitter<any> = new EventEmitter<any>();
    @Output() selectionChange: EventEmitter<any> = new EventEmitter<any>();

    @ContentChildren(TemplateRef) templates: QueryList<any>;

    /**
     * Fields, which can not be hidden using columnsCustomizationTool
     * @type {any[]}
     */
    @Input() mandatoryCols: string[] = [];
    /**
     * Fields, that won`t be displayed unless user enables them using columnsCustomizationTool
     */
    @Input() hiddenCols: string[] = [];
    @Input() defaultColDef: ColDef;

    @Output() gridReady: EventEmitter<any> = new EventEmitter();

    currentGridWidth: number;

    /**
     * For row reselection functionality
     */
    private selectedRows: IRowNode[] = [];
    /**
     * identifier string array - if valueGetter uses different data than grid input data,
     * provide object property path as string using dot notation
     */
    @Input() compositeKey: string[] = [];

    protected currentFilterModel: any;
    protected columsCustomizationSubscription: Subscription;
    protected selectionChangedSubj: Subject<any[]> = new Subject<any[]>();
    gridApi: GridApi;

    columnDefs: ColDef[];
    itemsFrom = 0;
    itemsTo = 0;
    currentPageNumber = 0;
    itemsPerPageOptions = ['25', '50', '150', '250'];
    itemsPerPageNumber = '25';
    pages: number[] = [1];
    allPagesCount = 0;
    currentPageSize = 0;
    itemsCount = 0;
    private componentDestroyed: Subject<void> = new Subject<void>();

    constructor(
        private appUtilsService: AppUtilsService,
        private elementRef: ElementRef
    ) { }

    ngAfterViewInit(): void {
        if (!this.defaultColDef) {
            this.defaultColDef = {
                comparator: (valueA, valueB) => {
                    let result = 0;
                    valueA = this.appUtilsService.normalizeMixedDataValue(valueA);
                    valueB = this.appUtilsService.normalizeMixedDataValue(valueB);
                    if (valueA > valueB) {
                        result = 1;
                    } else if (valueA < valueB) {
                        result = -1;
                    }
                    return result;
                },
                resizable: true,
                sortable: true,
                filter: true,
                flex: 1,
                minWidth: 100,
            };
        }

        this.allColumnDefs.forEach((colDef) => {
            if (colDef.colId) {
                const colTemplateRef = this.getTemplateForColumn(colDef.colId);
                if (colTemplateRef) {
                    // Use modern AG Grid component API
                    colDef.cellRenderer = AgCellTemplateRendererComponent;
                    colDef.cellRendererParams = {
                        ngTemplate: colTemplateRef,
                    };
                }
            }
        });

        this.allColumnDefs.forEach((colDef: ColDef) => {
            colDef.tooltipValueGetter = (params: ITooltipParams) => {
                if (params.colDef && 'valueGetter' in params.colDef && params.colDef.valueGetter) {
                    const valueGetter = (params.colDef as ColDef).valueGetter;
                    if (typeof valueGetter === 'function') {
                        // Create a ValueGetterParams-like object from ITooltipParams
                        const valueGetterParams = {
                            ...params,
                            getValue: (field: string) => params.data?.[field]
                        };
                        return '' + valueGetter(valueGetterParams as any);
                    } else {
                        return '' + valueGetter;
                    }
                } else if (colDef.field && params.data) {
                    return params.data[colDef.field];
                }
                return '';
            };

            if (colDef.colId !== 'actions' && colDef.sortable === undefined) {
                colDef.sortable = true;
            }
        });

        setTimeout(() => {
            this.columnDefs = [];
            if (this.multiselection) {
                this.columnDefs = [
                    {
                        colId: 'selectionField',
                        headerName: '',
                        headerCheckboxSelection: this.headerCheckboxSelection,
                        checkboxSelection: true,
                        headerCheckboxSelectionFilteredOnly: true,
                        maxWidth: 66,
                        pinned: 'left',
                    },
                ];
            }
            this.columnDefs = this.columnDefs.concat(this.allColumnDefs);
        }, 100);
    }

    ngOnInit() {
        this.setGridOptions();

        if (!this.tableId) {
            throw new Error('Missing tableId');
        }
    }

    ngOnDestroy(): void {
        this.componentDestroyed.next();
    }

    returnTranslationKey(colDef: string): string {
        return this.doNotUppercaseTranslationKeys
            ? this.translationPropertyKeyPrefix + colDef
            : this.translationPropertyKeyPrefix + colDef.toUpperCase();
    }

    refreshView(): void {
        if (this.gridApi) {
            this.gridApi.setGridOption('rowData', this.rowData);
            this.gridApi.redrawRows();
        }
    }

    onRowDataChanged() {
        this.resetFilter();
        this.restoreSelection();
        this.fitSize();
    }

    /**
     * compositeKey Input must be provided to each agGrid table implementation,
     * otherwise row reselection after data change will not work
     */
    restoreSelection() {
        setTimeout(() => {
            if (this.selectedRows.length && this.compositeKey.length) {
                this.setSelection(this.compositeKey, this.selectedRows);
            }
        }, 0);
    }

    saveSelection() {
        if (this.compositeKey.length) {
            this.selectedRows = this.getSelectedRows();
        }
    }

    resetColumnsVisibility(): void {
        if (this.gridApi) {
            this.columnDefs.forEach((colDef: ColDef) => {
                if (colDef.colId) {
                    this.gridApi.setColumnsVisible([colDef.colId], !!!colDef.hide);
                }
            });
            this.fitSize();
        }
    }

    fitSize(): void {
        setTimeout(() => {
            const element = this.elementRef.nativeElement;
            if (
                this.gridApi &&
                element.offsetParent !== null &&
                element.offsetWidth > 0
            ) {
                // First autosize columns based on content
                this.gridApi.autoSizeAllColumns(false); // false = don't skip header

                // Then make columns fit the available width
                this.gridApi.sizeColumnsToFit();

                // Ensure grid refreshes properly
                this.gridApi.refreshCells({ force: true });
            }
        });
    }

    onGridReady(params) {
        this.gridApi = params.api;
        this.resetColumnsVisibility();

        // Force grid to recalculate size after DOM is stable
        setTimeout(() => {
            if (this.gridApi) {
                this.fitSize();
            }
        }, 100);

        this.gridReady.emit(event);
    }

    onPaginationChanged(params): void {
        const gridApi: GridApi = this.gridApi ? this.gridApi : params.api;

        this.itemsCount = gridApi.paginationGetRowCount();
        this.itemsFrom =
            gridApi.paginationGetCurrentPage() *
            gridApi.paginationGetPageSize() + 1;
        if (this.itemsFrom > this.itemsCount) {
            this.itemsFrom = this.itemsCount;
        }
        this.itemsTo =
            (gridApi.paginationGetCurrentPage() + 1) *
            gridApi.paginationGetPageSize();
        if (this.itemsTo > this.itemsCount) {
            this.itemsTo = this.itemsCount;
        }
        this.currentPageNumber = gridApi.paginationGetCurrentPage() + 1;
        this.allPagesCount = gridApi.paginationGetTotalPages();
        if (this.currentPageNumber > this.allPagesCount) {
            this.currentPageNumber = this.allPagesCount;
        }
        this.pages = [];
        for (let i = 0; i < this.allPagesCount; i++) {
            this.pages.push(i + 1);
        }
        this.currentPageSize = gridApi.paginationGetPageSize();
    }

    onGridSizeChanged(params: GridSizeChangedEvent) {
        this.currentGridWidth = params.clientWidth;
        this.gridApi = params.api;
        this.fitSize();
    }

    setRowData(): void {
        if (this.gridApi) {
            this.gridApi.setGridOption('rowData', this.rowData);
        }
    }

    protected getTemplateForColumn(colId: string): TemplateRef<any> {
        const template = this.templates.find((ref: any) => {
            // _declarationTContainer is a private member of TemplateRef, this may break in future versions of angular
            const candidate = ref._declarationTContainer?.localNames?.find(
                (name: string) => name === colId + 'ColumnTemplate'
            );
            return candidate !== undefined;
        });
        return template;
    }

    getSelectedRows(): any[] {
        if (this.gridApi) {
            return this.gridApi.getSelectedRows();
        } else {
            return [];
        }
    }

    clearSelection(): void {
        if (this.gridApi) {
            this.gridApi.deselectAll();
            this.selectedRows = [];
        }
    }

    setSelection(compositeKey: string[], savedNodesData: any[]): void {
        if (this.gridApi && compositeKey && compositeKey.length) {
            this.gridApi.forEachNode((node: IRowNode) => {
                const loadedNodesFilteredData: any[] = compositeKey.map((key) =>
                    this.resolvePath(node.data, key)
                );
                savedNodesData.forEach((savedNodeData) => {
                    const savedNodesFilteredData: any[] = compositeKey.map(
                        (k) => this.resolvePath(savedNodeData, k)
                    );
                    if (
                        loadedNodesFilteredData.join() ===
                        savedNodesFilteredData.join()
                    ) {
                        node.setSelected(true);
                    }
                });
            });
        } else {
            console.error('CompositeKey or grid not present');
        }
    }

    private resolvePath(dataObj: Object, key: string): any {
        const pathComponents: string[] = key.split('.');
        let resolvedVal = dataObj;
        pathComponents.forEach((pathComponent: string) => {
            if (resolvedVal[pathComponent]) {
                resolvedVal = resolvedVal[pathComponent];
            } else {
                console.error('Could not resolve object path');
                return undefined;
            }
        });
        return resolvedVal;
    }

    resetFilter() {
        if (this.gridApi && this.currentFilterModel) {
            this.gridApi.setFilterModel(this.currentFilterModel);
        }
    }

    saveFilterModel() {
        if (this.gridApi) {
            this.currentFilterModel = this.gridApi.getFilterModel();
            this.fitSize();
        }
    }

    quickSearch(value: string) {
        if (this.gridApi) {
            this.gridApi.setGridOption('quickFilterText', value);
        }
    }

    clearQuickSearchBar() {
        if (this.gridApi) {
            this.gridApi.setGridOption('quickFilterText', '');
        }
    }

    onSelectionChanged(event: any) {
        this.saveSelection();
        this.selectionChangedSubj.next(event.api.getSelectedRows());
    }

    getSelectionChangedObservable(): Observable<any[]> {
        return this.selectionChangedSubj.asObservable();
    }

    onRowClicked(event: any) {
        this.rowClicked.emit(event);
    }

    onFirstPageClick() {
        this.gridApi.paginationGoToFirstPage();
        this.fitSize();
    }

    onCellMouseOver(event: any) {
        this.cellMouseOver.emit(event);
    }

    onPrevPageClick() {
        this.gridApi.paginationGoToPreviousPage();
        this.fitSize();
    }

    onCellMouseOut(event: any) {
        this.cellMouseOut.emit(event);
    }

    onNextPageClick() {
        this.gridApi.paginationGoToNextPage();
        this.fitSize();
    }

    onLastPageClick() {
        this.gridApi.paginationGoToLastPage();
        this.fitSize();
    }

    onPageSelectionChanged(changeEvent: MatSelectChange) {
        this.gridApi.paginationGoToPage(changeEvent.value - 1);
        this.fitSize();
    }

    resetRowHeights(): void {
        this.gridApi.resetRowHeights();
    }

    onItemsPerPageSelectionChanged(changeEvent: MatSelectChange) {
        this.gridApi.setGridOption('paginationPageSize', 1 * changeEvent.value);
        this.fitSize();
    }

    onFilterChanged(): void {
        if (this.gridApi) {
            this.gridApi.onFilterChanged();
        }
    }

    getFrameworkComponentInstance(filterInstanceId: string): any {
        if (this.gridApi) {
            const filterInstance = this.gridApi.getColumnFilterInstance(filterInstanceId);
            if (filterInstance && (filterInstance as any).getFrameworkComponentInstance) {
                return (filterInstance as any).getFrameworkComponentInstance();
            }
        }
    }

    private setGridOptions() {
        if (!this.gridOptions) {
            this.gridOptions = {
                suppressColumnVirtualisation: true,
                rowBuffer: 25,
                enableCellTextSelection: true,
                ensureDomOrder: true,
                theme: 'legacy',
                suppressCellFocus: true,
                // Add columnTypes for common column configurations
                columnTypes: {
                    nonEditableColumn: { editable: false },
                    numericColumn: {
                        filter: 'agNumberColumnFilter',
                        filterParams: {
                            filterOptions: ['equals', 'lessThan', 'greaterThan'],
                            suppressAndOrCondition: true
                        }
                    }
                }
            };
        } else {
            this.gridOptions['suppressColumnVirtualisation'] = true;
            this.gridOptions['rowBuffer'] = 25;
            this.gridOptions['enableCellTextSelection'] = true;
            this.gridOptions['ensureDomOrder'] = true;
            this.gridOptions['theme'] = 'legacy';
            this.gridOptions['suppressCellFocus'] = true;

            // Add column types if not present
            if (!this.gridOptions.columnTypes) {
                this.gridOptions.columnTypes = {
                    nonEditableColumn: { editable: false },
                    numericColumn: {
                        filter: 'agNumberColumnFilter',
                        filterParams: {
                            filterOptions: ['equals', 'lessThan', 'greaterThan'],
                            suppressAndOrCondition: true
                        }
                    }
                };
            }
        }

        // Add getRowId function if compositeKey is defined
        if (this.compositeKey && this.compositeKey.length) {
            this.gridOptions.getRowId = (params: GetRowIdParams) => {
                return this.compositeKey.map(key => this.resolvePath(params.data, key)).join('_');
            };
        }
    }
}
