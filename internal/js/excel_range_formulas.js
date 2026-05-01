// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  range.load(['address', 'rowCount', 'columnCount', 'values', 'formulas', 'formulasR1C1']);
  await context.sync();
  const total = range.rowCount * range.columnCount;
  const truncated = total > args.maxCells;
  return {
    address: range.address,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
    values: __sliceGrid(range.values, truncated),
    formulas: __sliceGrid(range.formulas, truncated),
    formulasR1C1: __sliceGrid(range.formulasR1C1, truncated),
    truncated,
  };
});
return { result: data };
