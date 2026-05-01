// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const ws = context.workbook.worksheets.getItem(args.sheet);
  const chart = ws.charts.getItem(args.name);
  chart.load([
    'id',
    'name',
    'chartType',
    'title/text',
    'left',
    'top',
    'width',
    'height',
    'axes/categoryAxis/title/text',
    'axes/valueAxis/title/text',
  ]);
  const series = chart.series;
  series.load('items/name,items/chartType');
  await context.sync();
  return {
    id: chart.id,
    name: chart.name,
    chartType: chart.chartType,
    title: chart.title ? chart.title.text : null,
    left: chart.left,
    top: chart.top,
    width: chart.width,
    height: chart.height,
    categoryAxisTitle:
      chart.axes && chart.axes.categoryAxis && chart.axes.categoryAxis.title
        ? chart.axes.categoryAxis.title.text
        : null,
    valueAxisTitle:
      chart.axes && chart.axes.valueAxis && chart.axes.valueAxis.title
        ? chart.axes.valueAxis.title.text
        : null,
    series: series.items.map((s) => ({ name: s.name, chartType: s.chartType })),
  };
});
return { result: data };
