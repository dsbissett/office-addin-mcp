// @requires ExcelApi 1.6
const data = await __runExcel(async (context) => {
  let range;
  if (!args.address) {
    const ws = args.sheet
      ? context.workbook.worksheets.getItem(args.sheet)
      : context.workbook.worksheets.getActiveWorksheet();
    range = ws.getUsedRangeOrNullObject(true);
  } else {
    range = __resolveRange(context, args.address, args.sheet);
  }
  const cfs = range.conditionalFormats;
  cfs.load('items/id,items/type,items/priority,items/stopIfTrue');
  range.load(['address', 'isNullObject']);
  await context.sync();
  if (range.isNullObject) {
    return { empty: true };
  }
  return {
    empty: false,
    address: range.address,
    conditionalFormats: cfs.items.map((c) => ({
      id: c.id,
      type: c.type,
      priority: c.priority,
      stopIfTrue: c.stopIfTrue,
    })),
  };
});
return { result: data };
