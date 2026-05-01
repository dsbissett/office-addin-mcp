// @requires ExcelApi 1.3
const data = await __runExcel(async (context) => {
  const pivots = context.workbook.pivotTables;
  pivots.load(
    'items/id,items/name,items/enableDataValueEditing,items/useCustomSortLists,items/worksheet/name',
  );
  await context.sync();
  const ranges = pivots.items.map((p) => p.layout.getRange().load('address'));
  await context.sync();
  return {
    pivotTables: pivots.items.map((p, i) => ({
      id: p.id,
      name: p.name,
      worksheet: p.worksheet.name,
      address: ranges[i].address,
      enableDataValueEditing: p.enableDataValueEditing,
      useCustomSortLists: p.useCustomSortLists,
    })),
  };
});
return { result: data };
