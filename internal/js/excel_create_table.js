// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const sheet = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  sheet.load('name');
  const hasHeaders = args.hasHeaders !== false;
  const table = sheet.tables.add(args.address, hasHeaders);
  if (typeof args.name === 'string' && args.name.length > 0) {
    table.name = args.name;
  }
  table.load(['name', 'id']);
  table.getRange().load('address');
  await context.sync();
  return {
    sheet: sheet.name,
    name: table.name,
    id: table.id,
    address: table.getRange().address,
    hasHeaders: hasHeaders,
  };
});
return { result: data };
