// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const wb = context.workbook;
  wb.load(['name', 'isDirty', 'readOnly']);
  const app = context.workbook.application;
  app.load(['calculationMode', 'calculationState']);
  const protection = wb.protection;
  protection.load('protected');
  await context.sync();
  return {
    name: wb.name,
    isDirty: wb.isDirty,
    readOnly: wb.readOnly,
    protected: protection.protected,
    calculationMode: app.calculationMode,
    calculationState: app.calculationState,
  };
});
return { result: data };
