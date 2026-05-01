// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const table = context.workbook.tables.getItem(args.name);
  table.load(['name', 'id', 'showHeaders', 'showTotals', 'style', 'worksheet/name']);
  const range = table.getRange().load('address');
  const body = table.getDataBodyRange().load('rowCount,columnCount');
  const cols = table.columns;
  cols.load('items/name,items/id,items/index');
  await context.sync();
  const filters = cols.items.map((c) => {
    const f = c.filter;
    f.load('criteria');
    return f;
  });
  await context.sync();
  return {
    name: table.name,
    id: table.id,
    worksheet: table.worksheet.name,
    address: range.address,
    rowCount: body.rowCount,
    columnCount: body.columnCount,
    showHeaders: table.showHeaders,
    showTotals: table.showTotals,
    style: table.style,
    columns: cols.items.map((c, i) => ({
      name: c.name,
      id: c.id,
      index: c.index,
      filterCriteria: filters[i].criteria,
    })),
  };
});
return { result: data };
