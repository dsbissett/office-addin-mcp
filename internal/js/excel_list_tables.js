// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const tables = context.workbook.tables;
  tables.load(
    'items/name,items/id,items/showHeaders,items/showTotals,items/style,items/worksheet/name',
  );
  await context.sync();
  const ranges = tables.items.map((t) => t.getRange().load('address'));
  const bodyRanges = tables.items.map((t) => t.getDataBodyRange().load('rowCount'));
  await context.sync();
  return {
    tables: tables.items.map((t, i) => ({
      name: t.name,
      id: t.id,
      worksheet: t.worksheet.name,
      address: ranges[i].address,
      rowCount: bodyRanges[i].rowCount,
      showHeaders: t.showHeaders,
      showTotals: t.showTotals,
      style: t.style,
    })),
  };
});
return { result: data };
