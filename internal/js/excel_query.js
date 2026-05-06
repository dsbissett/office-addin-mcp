// @requires ExcelApi 1.1
//
// Workflow tool: load a range, project values into row objects keyed by
// inferred or supplied headers, then run __queryEngine. Returns a small
// answer instead of round-tripping the full grid.
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  range.load(['address', 'rowCount', 'columnCount', 'values']);
  await context.sync();
  const total = range.rowCount * range.columnCount;
  const maxCells = args.maxCells || 500000;
  if (total > maxCells) {
    return {
      address: range.address,
      rowCount: range.rowCount,
      columnCount: range.columnCount,
      truncated: true,
      rows: null,
      count: 0,
    };
  }
  const grid = range.values || [];
  const rows = range.rowCount;
  const cols = range.columnCount;
  const headerMode = args.headers || 'first_row';

  let headers = [];
  let dataStart = 0;
  if (Array.isArray(headerMode)) {
    headers = headerMode.map((h) => String(h));
  } else if (headerMode === 'first_row' && rows > 0) {
    headers = (grid[0] || []).map((v) => v == null ? '' : String(v));
    dataStart = 1;
  } else {
    for (let c = 0; c < cols; c++) headers.push('col' + (c + 1));
  }

  const records = [];
  for (let r = dataStart; r < rows; r++) {
    const row = grid[r] || [];
    const obj = {};
    for (let c = 0; c < cols; c++) obj[headers[c] || ('col' + (c + 1))] = row[c] == null ? null : row[c];
    records.push(obj);
  }
  const result = __queryEngine(records, args.query || {});
  return {
    address: range.address,
    rowCount: rows,
    columnCount: cols,
    headers: headers,
    truncated: false,
    rows: result.rows,
    count: result.count,
    limited: result.truncated,
  };
});
return { result: data };
