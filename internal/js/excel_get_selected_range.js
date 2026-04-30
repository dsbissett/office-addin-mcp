// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const range = context.workbook.getSelectedRange();
  range.load(['address', 'values', 'rowCount', 'columnCount', 'worksheet/name']);
  await context.sync();
  return {
    sheet: range.worksheet.name,
    address: range.address,
    values: range.values,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
  };
});
return { result: data };
