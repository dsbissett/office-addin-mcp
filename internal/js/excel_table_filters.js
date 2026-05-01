// @requires ExcelApi 1.2
const data = await __runExcel(async (context) => {
  const table = context.workbook.tables.getItem(args.name);
  const cols = table.columns;
  cols.load('items/name,items/index');
  await context.sync();
  const filters = cols.items.map((c) => {
    const f = c.filter;
    f.load('criteria');
    return f;
  });
  await context.sync();
  return {
    table: args.name,
    columns: cols.items.map((c, i) => ({
      name: c.name,
      index: c.index,
      criteria: filters[i].criteria,
    })),
  };
});
return { result: data };
