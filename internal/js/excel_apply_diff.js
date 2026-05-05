// @requires ExcelApi 1.1
//
// Workflow tool: apply a list of {address, value?, formula?, numberFormat?}
// patches in one Excel.run. One CDP round-trip + one context.sync per call,
// regardless of patch count — replaces N back-and-forth writeRange calls.
const data = await __runExcel(async (context) => {
  const patches = Array.isArray(args.patches) ? args.patches : [];
  const results = [];
  const handles = [];
  for (let i = 0; i < patches.length; i++) {
    const p = patches[i];
    if (!p || typeof p.address !== 'string' || !p.address) {
      throw __officeError('invalid_patch', 'patch[' + i + '].address is required.');
    }
    const range = __resolveRange(context, p.address, p.sheet);
    if (Array.isArray(p.values)) range.values = p.values;
    else if ('value' in p) range.values = [[p.value]];
    if (Array.isArray(p.formulas)) range.formulas = p.formulas;
    else if (typeof p.formula === 'string') range.formulas = [[p.formula]];
    if (typeof p.numberFormat === 'string') {
      // Single format applied uniformly via load + sync would force an extra
      // round-trip; defer until after first sync.
      handles.push({ idx: i, range: range, numberFormat: p.numberFormat });
    } else if (Array.isArray(p.numberFormat)) {
      range.numberFormat = p.numberFormat;
    }
    range.load(['address', 'rowCount', 'columnCount']);
    results.push({ idx: i, range: range });
  }
  await context.sync();
  // Second pass for scalar numberFormat (now we know rowCount/columnCount).
  if (handles.length > 0) {
    for (let h = 0; h < handles.length; h++) {
      const fr = handles[h];
      const fmt = [];
      for (let r = 0; r < fr.range.rowCount; r++) {
        const row = [];
        for (let c = 0; c < fr.range.columnCount; c++) row.push(fr.numberFormat);
        fmt.push(row);
      }
      fr.range.numberFormat = fmt;
    }
    await context.sync();
  }
  return {
    applied: results.map((r) => ({
      address: r.range.address,
      rowCount: r.range.rowCount,
      columnCount: r.range.columnCount,
    })),
  };
});
return { result: data };
