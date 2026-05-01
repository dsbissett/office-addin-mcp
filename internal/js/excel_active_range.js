// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const range = context.workbook.getSelectedRange();
  const props = ['address', 'values', 'rowCount', 'columnCount'];
  if (args.includeFormulas) props.push('formulas');
  if (args.includeNumberFormat) props.push('numberFormat');
  range.load(props);
  await context.sync();
  const total = range.rowCount * range.columnCount;
  const truncated = total > args.maxCells;
  return {
    address: range.address,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
    values: __sliceGrid(range.values, truncated),
    formulas: args.includeFormulas ? __sliceGrid(range.formulas, truncated) : null,
    numberFormat: args.includeNumberFormat ? __sliceGrid(range.numberFormat, truncated) : null,
    truncated,
  };
});
return { result: data };
