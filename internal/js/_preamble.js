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
  if (typeof globalThis.Excel === 'undefined') {
    throw __officeError('excel_unavailable', 'Excel global is not available in this target.');
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
