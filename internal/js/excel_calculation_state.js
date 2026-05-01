// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const app = context.workbook.application;
  app.load([
    'calculationMode',
    'calculationState',
    'calculationEngineVersion',
    'iterativeCalculation',
  ]);
  await context.sync();
  return {
    calculationMode: app.calculationMode,
    calculationState: app.calculationState,
    calculationEngineVersion: app.calculationEngineVersion,
    iterativeCalculation: app.iterativeCalculation,
  };
});
return { result: data };
