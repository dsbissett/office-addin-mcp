// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const ws = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  const valuesOnly = args.valuesOnly !== false;
  const used = ws.getUsedRangeOrNullObject(valuesOnly);
  const props = ['address', 'values', 'rowCount', 'columnCount'];
  if (args.includeFormulas) props.push('formulas');
  if (args.includeNumberFormat) props.push('numberFormat');
  used.load(props);
  await context.sync();
  if (used.isNullObject) {
    return { empty: true };
  }
  const total = used.rowCount * used.columnCount;
  const truncated = total > args.maxCells;
  return {
    empty: false,
    address: used.address,
    rowCount: used.rowCount,
    columnCount: used.columnCount,
    values: __sliceGrid(used.values, truncated),
    formulas: args.includeFormulas ? __sliceGrid(used.formulas, truncated) : null,
    numberFormat: args.includeNumberFormat ? __sliceGrid(used.numberFormat, truncated) : null,
    truncated,
  };
});
return { result: data };
