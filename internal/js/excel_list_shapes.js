// @requires ExcelApi 1.9
const data = await __runExcel(async (context) => {
  const ws = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  ws.load('name');
  const shapes = ws.shapes;
  shapes.load(
    'items/id,items/name,items/type,items/left,items/top,items/width,items/height,items/visible,items/altTextDescription',
  );
  await context.sync();
  return {
    worksheet: ws.name,
    shapes: shapes.items.map((s) => ({
      id: s.id,
      name: s.name,
      type: s.type,
      left: s.left,
      top: s.top,
      width: s.width,
      height: s.height,
      visible: s.visible,
      altTextDescription: s.altTextDescription,
    })),
  };
});
return { result: data };
