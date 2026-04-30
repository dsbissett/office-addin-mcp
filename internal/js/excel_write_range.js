// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const sheet = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  sheet.load('name');
  const range = sheet.getRange(args.address);
  if (Array.isArray(args.values)) {
    range.values = args.values;
  }
  if (Array.isArray(args.formulas)) {
    range.formulas = args.formulas;
  }
  if (typeof args.numberFormat === 'string') {
    range.load(['rowCount', 'columnCount']);
    await context.sync();
    const fmt = [];
    for (let r = 0; r < range.rowCount; r++) {
      const row = [];
      for (let c = 0; c < range.columnCount; c++) row.push(args.numberFormat);
      fmt.push(row);
    }
    range.numberFormat = fmt;
  } else if (Array.isArray(args.numberFormat)) {
    range.numberFormat = args.numberFormat;
  }
  range.load(['address', 'rowCount', 'columnCount']);
  await context.sync();
  return {
    sheet: sheet.name,
    address: range.address,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
  };
});
return { result: data };
