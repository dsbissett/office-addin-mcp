// @requires ExcelApi 1.3
const data = await __runExcel(async (context) => {
  const pt = context.workbook.pivotTables.getItem(args.name);
  pt.load(['id', 'name', 'worksheet/name']);
  const rows = pt.rowHierarchies;
  const cols = pt.columnHierarchies;
  const dataH = pt.dataHierarchies;
  const filters = pt.filterHierarchies;
  rows.load('items/id,items/name');
  cols.load('items/id,items/name');
  dataH.load('items/id,items/name,items/summarizeBy,items/showAs,items/numberFormat');
  filters.load('items/id,items/name');
  const layoutRange = pt.layout.getRange().load('address');
  await context.sync();
  return {
    id: pt.id,
    name: pt.name,
    worksheet: pt.worksheet.name,
    address: layoutRange.address,
    rowHierarchies: rows.items.map((h) => ({ id: h.id, name: h.name })),
    columnHierarchies: cols.items.map((h) => ({ id: h.id, name: h.name })),
    dataHierarchies: dataH.items.map((h) => ({
      id: h.id,
      name: h.name,
      summarizeBy: h.summarizeBy,
      showAs: h.showAs,
      numberFormat: h.numberFormat,
    })),
    filterHierarchies: filters.items.map((h) => ({ id: h.id, name: h.name })),
  };
});
return { result: data };
