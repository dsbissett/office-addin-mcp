// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const sheet = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  sheet.load('name');
  const range = sheet.getRange(args.address);
  range.load([
    'address',
    'values',
    'formulas',
    'numberFormat',
    'rowCount',
    'columnCount',
  ]);
  await context.sync();
  return {
    sheet: sheet.name,
    address: range.address,
    values: range.values,
    formulas: range.formulas,
    numberFormat: range.numberFormat,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
  };
});
return { result: data };
