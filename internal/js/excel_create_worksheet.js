// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const s = context.workbook.worksheets.add(args.name);
  s.load(['name', 'id', 'position']);
  await context.sync();
  return { name: s.name, id: s.id, position: s.position };
});
return { result: data };
