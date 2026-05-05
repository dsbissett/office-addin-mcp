// @requires ExcelApi 1.1
//
// Workflow tool: one-call workbook discovery — sheets, tables, named ranges,
// per-sheet used-range bounds. Replaces the old "list_worksheets +
// list_tables + list_named_items + per-sheet used_range" multi-step probe.
const data = await __runExcel(async (context) => {
  const sheets = context.workbook.worksheets;
  sheets.load(['items/name', 'items/id', 'items/position', 'items/visibility']);
  const tables = context.workbook.tables;
  tables.load(['items/name', 'items/id', 'items/showHeaders', 'items/showTotals', 'items/worksheet/name']);
  const named = context.workbook.names;
  named.load(['items/name', 'items/type', 'items/value', 'items/comment', 'items/scope']);
  await context.sync();

  const usedHandles = sheets.items.map((ws) => {
    const used = ws.getUsedRangeOrNullObject(true);
    used.load(['address', 'rowCount', 'columnCount']);
    return { name: ws.name, used: used };
  });
  await context.sync();

  return {
    worksheets: sheets.items.map((s, i) => ({
      name: s.name,
      id: s.id,
      position: s.position,
      visibility: s.visibility,
      usedRange: usedHandles[i].used.isNullObject
        ? null
        : {
            address: usedHandles[i].used.address,
            rowCount: usedHandles[i].used.rowCount,
            columnCount: usedHandles[i].used.columnCount,
          },
    })),
    tables: tables.items.map((t) => ({
      name: t.name,
      id: t.id,
      sheet: t.worksheet ? t.worksheet.name : null,
      showHeaders: t.showHeaders,
      showTotals: t.showTotals,
    })),
    namedRanges: named.items.map((n) => ({
      name: n.name,
      type: n.type,
      value: n.value,
      scope: n.scope,
      comment: n.comment,
    })),
  };
});
return { result: data };
