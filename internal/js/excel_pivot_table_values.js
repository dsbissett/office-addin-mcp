// @requires ExcelApi 1.3
const data = await __runExcel(async (context) => {
  const pt = context.workbook.pivotTables.getItem(args.name);
  const range = pt.layout.getRange();
  range.load(['address', 'values', 'rowCount', 'columnCount']);
  await context.sync();
  const total = range.rowCount * range.columnCount;
  const truncated = total > args.maxCells;
  return {
    address: range.address,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
    values: __sliceGrid(range.values, truncated),
    truncated,
  };
});
return { result: data };
