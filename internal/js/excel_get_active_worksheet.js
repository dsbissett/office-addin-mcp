// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const s = context.workbook.worksheets.getActiveWorksheet();
  s.load(['name', 'id', 'position', 'visibility']);
  await context.sync();
  return {
    name: s.name,
    id: s.id,
    position: s.position,
    visibility: s.visibility,
  };
});
return { result: data };
