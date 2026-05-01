// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const table = context.workbook.tables.getItem(args.name);
  const body = table.getDataBodyRange();
  body.load(['address', 'values', 'rowCount', 'columnCount']);
  let headers = null;
  if (args.includeHeaders) {
    headers = table.getHeaderRowRange().load('values');
  }
  await context.sync();
  const total = body.rowCount * body.columnCount;
  const truncated = total > args.maxCells;
  return {
    address: body.address,
    rowCount: body.rowCount,
    columnCount: body.columnCount,
    headers: args.includeHeaders && headers ? headers.values[0] : null,
    values: __sliceGrid(body.values, truncated),
    truncated,
  };
});
return { result: data };
