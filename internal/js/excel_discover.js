// @requires ExcelApi 1.1
//
// Discovery payload: returns workbook name, sheet list, used-range bounds
// per sheet, table catalog, named ranges, and a coarse fingerprint the Go
// side uses to invalidate cached snapshots when the workbook drifts.
const data = await __runExcel(async (context) => {
  const wb = context.workbook;
  wb.load(['name']);
  const sheets = wb.worksheets;
  sheets.load(['items/name', 'items/id', 'items/position', 'items/visibility']);
  const tables = wb.tables;
  tables.load(['items/name', 'items/id', 'items/showHeaders', 'items/showTotals', 'items/worksheet/name']);
  const named = wb.names;
  named.load(['items/name', 'items/type', 'items/value', 'items/scope']);
  await context.sync();

  const usedHandles = sheets.items.map((ws) => {
    const used = ws.getUsedRangeOrNullObject(true);
    used.load(['address', 'rowCount', 'columnCount']);
    return { name: ws.name, used: used };
  });
  await context.sync();

  let cellSum = 0;
  const worksheets = sheets.items.map((s, i) => {
    const u = usedHandles[i].used;
    const ur = u.isNullObject
      ? null
      : { address: u.address, rowCount: u.rowCount, columnCount: u.columnCount };
    if (ur) cellSum += (ur.rowCount || 0) * (ur.columnCount || 0);
    return {
      name: s.name,
      id: s.id,
      position: s.position,
      visibility: s.visibility,
      usedRange: ur,
    };
  });
  // Cheap fingerprint: sheets count + tables count + named count + total used cells.
  const fingerprint =
    'wb:' + sheets.items.length +
    ':t' + tables.items.length +
    ':n' + named.items.length +
    ':c' + cellSum;
  return {
    filePath: wb.name || '',
    fingerprint: fingerprint,
    worksheets: worksheets,
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
    })),
  };
});
return { result: data };
