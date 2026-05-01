// @requires ExcelApi 1.1
const data = await __runExcel(async (context) => {
  const worksheets = [];
  if (args.sheet) {
    const ws = context.workbook.worksheets.getItem(args.sheet);
    ws.load('name');
    worksheets.push(ws);
  } else {
    const list = context.workbook.worksheets;
    list.load('items/name');
    await context.sync();
    for (const ws of list.items) {
      worksheets.push(ws);
    }
  }
  const chartLists = worksheets.map((ws) => {
    const charts = ws.charts;
    charts.load(
      'items/id,items/name,items/chartType,items/title/text,items/left,items/top,items/width,items/height',
    );
    return { ws, charts };
  });
  await context.sync();
  const charts = [];
  for (const { ws, charts: list } of chartLists) {
    for (const c of list.items) {
      charts.push({
        id: c.id,
        name: c.name,
        worksheet: ws.name,
        chartType: c.chartType,
        title: c.title ? c.title.text : null,
        left: c.left,
        top: c.top,
        width: c.width,
        height: c.height,
      });
    }
  }
  return { charts };
});
return { result: data };
