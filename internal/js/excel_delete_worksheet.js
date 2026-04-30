// @requires ExcelApi 1.1
await __runExcel(async (context) => {
  const s = context.workbook.worksheets.getItem(args.name);
  s.delete();
  await context.sync();
});
return { result: { name: args.name, deleted: true } };
