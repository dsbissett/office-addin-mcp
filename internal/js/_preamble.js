// Common helpers concatenated before every Excel.* payload. The full payload
// is wrapped by the Go executor as:
//
//   (async (args) => {
//     try {
//       <PREAMBLE>
//       <PAYLOAD BODY>
//     } catch (e) { /* envelope conversion */ }
//   })(<ARGS_JSON>)
//
// Helpers prefixed with `__` are private to the wrapper. Payloads call
// __runExcel(fn) which ensures Office.js is ready and wraps Excel.run with
// structured error reporting.

function __officeError(code, message, extra) {
  const err = new Error(message);
  err.__officeError = true;
  err.code = code;
  err.message = message;
  if (extra && typeof extra === 'object') {
    if ('debugInfo' in extra) err.debugInfo = extra.debugInfo;
  }
  return err;
}

async function __ensureOffice() {
  if (typeof globalThis.Office === 'undefined') {
    throw __officeError('office_unavailable', 'Office.js is not loaded in this target.');
  }
  await Promise.race([
    new Promise((resolve, reject) => {
      try {
        Office.onReady(() => resolve());
      } catch (e) {
        reject(__officeError('office_ready_failed', String(e && e.message || e)));
      }
    }),
    new Promise((_, reject) =>
      setTimeout(
        () => reject(__officeError('office_ready_timeout', 'Office.onReady timed out after 1000ms')),
        1000,
      ),
    ),
  ]);
}

function __requireSet(name, version) {
  let supported = false;
  try {
    supported = Office.context.requirements.isSetSupported(name, version);
  } catch (e) {
    throw __officeError('requirement_check_failed', String(e && e.message || e));
  }
  if (!supported) {
    throw __officeError(
      'requirement_unmet',
      'Requirement set ' + name + '@' + version + ' is not supported by this host.',
    );
  }
}

function __resolveRange(context, address, sheet) {
  if (!address) {
    return context.workbook.getSelectedRange();
  }
  let sheetName = sheet;
  let a1 = address;
  const bangIdx = address.indexOf('!');
  if (bangIdx >= 0) {
    sheetName = address.slice(0, bangIdx).replace(/^'|'$/g, '');
    a1 = address.slice(bangIdx + 1);
  }
  const ws = sheetName
    ? context.workbook.worksheets.getItem(sheetName)
    : context.workbook.worksheets.getActiveWorksheet();
  return ws.getRange(a1);
}

function __sliceGrid(values, truncated) {
  if (!truncated || !Array.isArray(values)) return values;
  return values.slice(0, 1).map((row) => (Array.isArray(row) ? row.slice(0, 1) : row));
}

async function __runExcel(fn) {
  await __ensureOffice();
  try {
    return await Excel.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    const code = (e && e.code) || 'excel_run_failed';
    const message = (e && e.message) || String(e);
    const debugInfo = e && e.debugInfo;
    throw __officeError(code, message, { debugInfo });
  }
}

async function __runWord(fn) {
  await __ensureOffice();
  try {
    return await Word.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    const code = (e && e.code) || 'word_run_failed';
    const message = (e && e.message) || String(e);
    const debugInfo = e && e.debugInfo;
    throw __officeError(code, message, { debugInfo });
  }
}

async function __runPowerPoint(fn) {
  await __ensureOffice();
  try {
    return await PowerPoint.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    const code = (e && e.code) || 'powerpoint_run_failed';
    const message = (e && e.message) || String(e);
    const debugInfo = e && e.debugInfo;
    throw __officeError(code, message, { debugInfo });
  }
}

async function __runOneNote(fn) {
  await __ensureOffice();
  try {
    return await OneNote.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    const code = (e && e.code) || 'onenote_run_failed';
    const message = (e && e.message) || String(e);
    const debugInfo = e && e.debugInfo;
    throw __officeError(code, message, { debugInfo });
  }
}

async function __runOutlook(fn) {
  await __ensureOffice();
  try {
    return await fn(Office.context.mailbox);
  } catch (e) {
    if (e && e.__officeError) throw e;
    const code = (e && e.code) || 'outlook_run_failed';
    const message = (e && e.message) || String(e);
    const debugInfo = e && e.debugInfo;
    throw __officeError(code, message, { debugInfo });
  }
}

// __queryEngine evaluates a small JSON-shaped query DSL against an array of
// records. Used by the host-specific *.query payloads (excel, outlook,
// powerpoint, onenote). Shape:
//
//   { filter:  <jsonlogic-ish predicate>,
//     project: ["col1", "col2"],
//     groupBy: ["sku"],
//     agg:     [{ col: "qty", fn: "sum" | "count" | "avg" | "min" | "max" }],
//     limit:   <int> }
//
// Filter DSL is intentionally tiny: { "==": ["field", value] }, with
// supported ops eq/!=/</<=/>/>=, and/or/not, in, contains. Field references
// are bare strings; literals are anything else.
function __queryEngine(rows, q) {
  q = q || {};
  let out = Array.isArray(rows) ? rows.slice() : [];
  if (q.filter) {
    out = out.filter((r) => __qEval(q.filter, r));
  }
  if (Array.isArray(q.groupBy) && q.groupBy.length > 0) {
    const groups = new Map();
    for (let i = 0; i < out.length; i++) {
      const r = out[i];
      const key = q.groupBy.map((c) => r == null ? '' : r[c]).join('');
      let bucket = groups.get(key);
      if (!bucket) {
        bucket = { __keys: q.groupBy.map((c) => r == null ? null : r[c]), __rows: [] };
        groups.set(key, bucket);
      }
      bucket.__rows.push(r);
    }
    const aggSpecs = Array.isArray(q.agg) ? q.agg : [];
    out = [];
    for (const bucket of groups.values()) {
      const obj = {};
      for (let i = 0; i < q.groupBy.length; i++) obj[q.groupBy[i]] = bucket.__keys[i];
      for (const a of aggSpecs) {
        const name = a.as || (a.fn + '_' + a.col);
        obj[name] = __qAgg(bucket.__rows, a.col, a.fn);
      }
      out.push(obj);
    }
  } else if (Array.isArray(q.agg) && q.agg.length > 0) {
    const obj = {};
    for (const a of q.agg) {
      const name = a.as || (a.fn + '_' + a.col);
      obj[name] = __qAgg(out, a.col, a.fn);
    }
    out = [obj];
  }
  if (Array.isArray(q.project) && q.project.length > 0) {
    out = out.map((r) => {
      const o = {};
      for (const c of q.project) o[c] = r == null ? null : r[c];
      return o;
    });
  }
  const limit = typeof q.limit === 'number' ? q.limit : 0;
  const truncated = limit > 0 && out.length > limit;
  if (truncated) out = out.slice(0, limit);
  return { rows: out, count: out.length, truncated: truncated };
}

function __qEval(node, row) {
  if (node === null || node === undefined) return null;
  if (typeof node !== 'object' || Array.isArray(node)) return node;
  const keys = Object.keys(node);
  if (keys.length !== 1) return null;
  const op = keys[0];
  const args = node[op];
  switch (op) {
    case 'var': {
      const name = Array.isArray(args) ? args[0] : args;
      return row == null ? null : row[name];
    }
    case '==': return __qV(args[0], row) == __qV(args[1], row);
    case '!=': return __qV(args[0], row) != __qV(args[1], row);
    case '<':  return __qV(args[0], row) <  __qV(args[1], row);
    case '<=': return __qV(args[0], row) <= __qV(args[1], row);
    case '>':  return __qV(args[0], row) >  __qV(args[1], row);
    case '>=': return __qV(args[0], row) >= __qV(args[1], row);
    case 'and': return args.every((x) => !!__qEval(x, row));
    case 'or':  return args.some((x) => !!__qEval(x, row));
    case 'not': return !__qEval(args, row);
    case 'in': {
      const v = __qV(args[0], row);
      const list = __qV(args[1], row);
      return Array.isArray(list) && list.indexOf(v) >= 0;
    }
    case 'contains': {
      const haystack = __qV(args[0], row);
      const needle = __qV(args[1], row);
      if (typeof haystack !== 'string' || typeof needle !== 'string') return false;
      return haystack.indexOf(needle) >= 0;
    }
    default: return null;
  }
}

function __qV(node, row) {
  if (node && typeof node === 'object' && !Array.isArray(node)) return __qEval(node, row);
  if (typeof node === 'string' && row && Object.prototype.hasOwnProperty.call(row, node)) return row[node];
  return node;
}

function __qAgg(rows, col, fn) {
  if (fn === 'count') return rows.length;
  let n = 0; let sum = 0; let mn = null; let mx = null;
  for (let i = 0; i < rows.length; i++) {
    const v = rows[i] == null ? null : rows[i][col];
    if (typeof v !== 'number' || !isFinite(v)) continue;
    n++; sum += v;
    if (mn === null || v < mn) mn = v;
    if (mx === null || v > mx) mx = v;
  }
  switch (fn) {
    case 'sum': return sum;
    case 'avg': return n > 0 ? sum / n : null;
    case 'min': return mn;
    case 'max': return mx;
    default: return null;
  }
}
