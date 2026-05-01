// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const ws = context.workbook.worksheets.getItem(args.sheet);
  const chart = ws.charts.getItem(args.name);
  const image =
    args.width && args.height ? chart.getImage(args.width, args.height) : chart.getImage();
  await context.sync();
  return {
    sheet: args.sheet,
    name: args.name,
    mimeType: 'image/png',
    data: image.value,
  };
});
return { result: data };
