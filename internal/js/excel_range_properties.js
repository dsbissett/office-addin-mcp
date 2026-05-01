// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  const props = [
    'address',
    'rowCount',
    'columnCount',
    'valueTypes',
    'hasSpill',
    'rowHidden',
    'columnHidden',
  ];
  if (args.includeStyle) props.push('style');
  range.load(props);
  if (args.includeFormat) {
    range.format.load(['horizontalAlignment', 'verticalAlignment', 'wrapText']);
    range.format.font.load(['name', 'size', 'bold', 'italic', 'color']);
    range.format.fill.load('color');
  }
  await context.sync();
  const total = range.rowCount * range.columnCount;
  const truncated = total > args.maxCells;
  return {
    address: range.address,
    rowCount: range.rowCount,
    columnCount: range.columnCount,
    valueTypes: __sliceGrid(range.valueTypes, truncated),
    hasSpill: range.hasSpill,
    rowHidden: range.rowHidden,
    columnHidden: range.columnHidden,
    style: args.includeStyle ? range.style : null,
    format: args.includeFormat
      ? {
          horizontalAlignment: range.format.horizontalAlignment,
          verticalAlignment: range.format.verticalAlignment,
          wrapText: range.format.wrapText,
          font: {
            name: range.format.font.name,
            size: range.format.font.size,
            bold: range.format.font.bold,
            italic: range.format.font.italic,
            color: range.format.font.color,
          },
          fill: { color: range.format.fill.color },
        }
      : null,
    truncated,
  };
});
return { result: data };
