// @requires ExcelApi 1.1
//
// Workflow tool: load a range, infer headers + column types, and return a
// typed table the agent can reason over without re-reading raw values. Replaces
// the manual "readRange + look at first row + guess types" loop with one call.
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  range.load(['address', 'rowCount', 'columnCount', 'values']);
  await context.sync();
  const total = range.rowCount * range.columnCount;
  const truncated = total > (args.maxCells || 100000);
  if (truncated) {
    return {
      address: range.address,
      rowCount: range.rowCount,
      columnCount: range.columnCount,
      truncated: true,
      headers: null,
      columnTypes: null,
      rows: null,
    };
  }
  const grid = range.values || [];
  const rows = range.rowCount;
  const cols = range.columnCount;
  const headerMode = args.headers || 'auto'; // 'first_row' | 'none' | 'auto'

  let headers = null;
  let dataStart = 0;
  if (rows > 0) {
    if (headerMode === 'first_row' || (headerMode === 'auto' && __looksLikeHeaderRow(grid[0], grid[1]))) {
      headers = grid[0].map((v) => (v == null ? '' : String(v)));
      dataStart = 1;
    } else {
      headers = [];
      for (let c = 0; c < cols; c++) headers.push('col' + (c + 1));
    }
  } else {
    headers = [];
  }

  const columnTypes = new Array(cols).fill('empty');
  const tally = new Array(cols);
  for (let c = 0; c < cols; c++) tally[c] = { number: 0, string: 0, boolean: 0, empty: 0, date: 0 };
  for (let r = dataStart; r < rows; r++) {
    const row = grid[r] || [];
    for (let c = 0; c < cols; c++) {
      const v = row[c];
      if (v === null || v === undefined || v === '') tally[c].empty++;
      else if (typeof v === 'number') tally[c].number++;
      else if (typeof v === 'boolean') tally[c].boolean++;
      else if (typeof v === 'string') {
        if (__looksLikeDate(v)) tally[c].date++;
        else tally[c].string++;
      } else tally[c].string++;
    }
  }
  for (let c = 0; c < cols; c++) {
    const t = tally[c];
    const nonEmpty = t.number + t.string + t.boolean + t.date;
    if (nonEmpty === 0) columnTypes[c] = 'empty';
    else if (t.number === nonEmpty) columnTypes[c] = 'number';
    else if (t.boolean === nonEmpty) columnTypes[c] = 'boolean';
    else if (t.date === nonEmpty) columnTypes[c] = 'date';
    else if (t.string + t.date === nonEmpty) columnTypes[c] = 'string';
    else columnTypes[c] = 'mixed';
  }

  const outRows = [];
  for (let r = dataStart; r < rows; r++) {
    const row = grid[r] || [];
    const obj = {};
    for (let c = 0; c < cols; c++) {
      obj[headers[c] || 'col' + (c + 1)] = row[c] == null ? null : row[c];
    }
    outRows.push(obj);
  }
  return {
    address: range.address,
    rowCount: rows,
    columnCount: cols,
    truncated: false,
    headers: headers,
    columnTypes: columnTypes,
    rows: outRows,
  };
});
return { result: data };

function __looksLikeHeaderRow(first, second) {
  if (!Array.isArray(first)) return false;
  // Header heuristic: first row is all strings, second row has at least one
  // non-string value (number / boolean). Skips the empty workbook edge case.
  let allStrings = true;
  for (let i = 0; i < first.length; i++) {
    const v = first[i];
    if (v == null || v === '') return false;
    if (typeof v !== 'string') { allStrings = false; break; }
  }
  if (!allStrings) return false;
  if (!Array.isArray(second)) return true;
  for (let i = 0; i < second.length; i++) {
    const v = second[i];
    if (typeof v === 'number' || typeof v === 'boolean') return true;
  }
  return false;
}

function __looksLikeDate(s) {
  if (typeof s !== 'string' || s.length < 6 || s.length > 32) return false;
  // Cheap guard — agent receives 'string' for dates that don't match this.
  return /^\d{4}[-/]\d{1,2}[-/]\d{1,2}/.test(s) || /^\d{1,2}[-/]\d{1,2}[-/]\d{2,4}/.test(s);
}
