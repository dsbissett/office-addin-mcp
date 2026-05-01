// @requires ExcelApi 1.4
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  const valueType = args.valueType || 'all';
  const special =
    args.cellType === 'blanks' || args.cellType === 'visible'
      ? range.getSpecialCellsOrNullObject(args.cellType)
      : range.getSpecialCellsOrNullObject(args.cellType, valueType);
  special.load(['address', 'cellCount', 'isNullObject']);
  await context.sync();
  if (special.isNullObject) {
    return { found: false };
  }
  return {
    found: true,
    address: special.address,
    cellCount: special.cellCount,
  };
});
return { result: data };
