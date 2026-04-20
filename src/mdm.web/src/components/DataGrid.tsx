import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { AgGridReact } from "ag-grid-react";
import type { AgGridReactProps } from "ag-grid-react";
import { themeQuartz, colorSchemeDark, colorSchemeLight, type ColDef } from "ag-grid-enterprise";

// Minimal locale dictionaries. AG Grid ships richer locale files on NPM
// (@ag-grid-community/locale) but for our modest string surface we inline
// just the keys actually rendered.
const LOCALE_ZH_TW: Record<string, string> = {
  // generic
  page: "頁",
  to: "至",
  of: "共",
  next: "下一頁",
  previous: "上一頁",
  first: "第一頁",
  last: "最後一頁",
  pageLastRowUnknown: "?",
  pageSizeSelectorLabel: "每頁筆數：",
  loadingOoo: "載入中…",
  noRowsToShow: "無資料",
  // filter
  searchOoo: "搜尋…",
  selectAll: "全選",
  selectAllSearchResults: "全選搜尋結果",
  addCurrentSelectionToFilter: "加入目前選取",
  blanks: "(空白)",
  equals: "等於",
  notEqual: "不等於",
  contains: "包含",
  notContains: "不包含",
  startsWith: "開頭為",
  endsWith: "結尾為",
  greaterThan: "大於",
  greaterThanOrEqual: "大於或等於",
  lessThan: "小於",
  lessThanOrEqual: "小於或等於",
  inRange: "介於",
  filterOoo: "篩選…",
  applyFilter: "套用",
  resetFilter: "重設",
  clearFilter: "清除",
  cancelFilter: "取消",
  // columns tool panel
  columns: "欄位",
  filters: "篩選器",
  pivots: "樞紐",
  // menu
  pinColumn: "釘選欄位",
  pinLeft: "釘選至左",
  pinRight: "釘選至右",
  noPin: "取消釘選",
  valueAggregation: "彙總",
  autosizeThiscolumn: "自動調整此欄寬",
  autosizeAllColumns: "自動調整所有欄寬",
  groupBy: "依此欄分組",
  ungroupBy: "取消分組",
  resetColumns: "重設欄位",
  expandAll: "全部展開",
  collapseAll: "全部收合",
  copy: "複製",
  copyWithHeaders: "含標題複製",
  copyWithGroupHeaders: "含群組標題複製",
  paste: "貼上",
  export: "匯出",
  csvExport: "CSV 匯出",
  excelExport: "Excel 匯出",
  // row group
  rowGroupColumnsEmptyMessage: "拖曳欄位到此處進行分組",
  valueColumnsEmptyMessage: "拖曳欄位到此處進行彙總",
  pivotColumnsEmptyMessage: "拖曳欄位到此處作為樞紐欄位",
  // status bar
  totalAndFilteredRows: "列",
  totalRows: "合計列數",
  filteredRows: "已篩選",
  selectedRows: "已選取",
  averageValue: "平均",
  sumValue: "加總",
  minValue: "最小",
  maxValue: "最大",
  countValue: "筆數",
};

const LOCALE_EN: Record<string, string> = {};

interface DataGridProps<T> extends Omit<AgGridReactProps<T>, "theme"> {
  /** CSS height — defaults to a comfortable full-card height. */
  height?: string | number;
  /** Disable the sidebar tool panels (columns/filters). */
  hideSidebar?: boolean;
}

export function DataGrid<T>({
  height = "calc(100vh - 16rem)",
  hideSidebar,
  defaultColDef,
  rowSelection,
  pagination = true,
  paginationPageSize = 50,
  paginationPageSizeSelector = [20, 50, 100, 200],
  sideBar,
  ...rest
}: DataGridProps<T>) {
  const { i18n } = useTranslation();
  const [isDark, setIsDark] = useState(
    () => document.documentElement.getAttribute("data-theme") === "dark",
  );
  const observerRef = useRef<MutationObserver | null>(null);

  // Track <html data-theme> changes so the grid re-themes live with the app.
  useEffect(() => {
    const el = document.documentElement;
    const check = () => setIsDark(el.getAttribute("data-theme") === "dark");
    check();
    observerRef.current = new MutationObserver(check);
    observerRef.current.observe(el, { attributes: true, attributeFilter: ["data-theme"] });
    return () => observerRef.current?.disconnect();
  }, []);

  const theme = useMemo(
    () =>
      themeQuartz.withPart(isDark ? colorSchemeDark : colorSchemeLight).withParams({
        fontFamily: "inherit",
        borderRadius: 6,
        headerHeight: 38,
        rowHeight: 36,
      }),
    [isDark],
  );

  const mergedDefaultColDef: ColDef = useMemo(
    () => ({
      sortable: true,
      filter: true,
      resizable: true,
      floatingFilter: false,
      minWidth: 80,
      flex: 1,
      ...defaultColDef,
    }),
    [defaultColDef],
  );

  const resolvedSidebar =
    sideBar !== undefined
      ? sideBar
      : hideSidebar
        ? false
        : {
            toolPanels: [
              {
                id: "columns",
                labelDefault: "Columns",
                labelKey: "columns",
                iconKey: "columns",
                toolPanel: "agColumnsToolPanel",
                toolPanelParams: { suppressRowGroups: true, suppressValues: true, suppressPivots: true },
              },
              {
                id: "filters",
                labelDefault: "Filters",
                labelKey: "filters",
                iconKey: "filter",
                toolPanel: "agFiltersToolPanel",
              },
            ],
          };

  const localeText = i18n.language === "en" ? LOCALE_EN : LOCALE_ZH_TW;

  return (
    <div style={{ width: "100%", height }}>
      <AgGridReact<T>
        theme={theme}
        defaultColDef={mergedDefaultColDef}
        rowSelection={rowSelection}
        pagination={pagination}
        paginationPageSize={paginationPageSize}
        paginationPageSizeSelector={paginationPageSizeSelector}
        sideBar={resolvedSidebar}
        localeText={localeText}
        animateRows
        suppressCellFocus
        {...rest}
      />
    </div>
  );
}
