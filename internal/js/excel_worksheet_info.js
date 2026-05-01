// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const ws = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  ws.load([
    'name',
    'id',
    'position',
    'visibility',
    'tabColor',
    'showGridlines',
    'showHeadings',
    'standardHeight',
    'standardWidth',
  ]);
  const used = ws.getUsedRangeOrNullObject(true);
  used.load('address');
  const protection = ws.protection;
  protection.load('protected');
  await context.sync();
  return {
    name: ws.name,
    id: ws.id,
    position: ws.position,
    visibility: ws.visibility,
    tabColor: ws.tabColor,
    showGridlines: ws.showGridlines,
    showHeadings: ws.showHeadings,
    standardHeight: ws.standardHeight,
    standardWidth: ws.standardWidth,
    protected: protection.protected,
    usedRangeAddress: used.isNullObject ? null : used.address,
  };
});
return { result: data };
