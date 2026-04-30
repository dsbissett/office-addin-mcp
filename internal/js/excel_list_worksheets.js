// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const sheets = context.workbook.worksheets;
  sheets.load(['items/name', 'items/id', 'items/position', 'items/visibility']);
  await context.sync();
  return {
    worksheets: sheets.items.map((s) => ({
      name: s.name,
      id: s.id,
      position: s.position,
      visibility: s.visibility,
    })),
  };
});
return { result: data };
