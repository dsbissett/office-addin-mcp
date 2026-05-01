// Open an Office Dialog API dialog. Must be invoked from a target that has
// access to Office.context.ui (typically the taskpane). Returns once
// displayDialogAsync resolves; the caller can then locate the dialog target
// via cdp targets / addin.listTargets.
await __ensureOffice();

if (!Office.context || !Office.context.ui || typeof Office.context.ui.displayDialogAsync !== 'function') {
  throw __officeError('dialog_unavailable', 'Office.context.ui.displayDialogAsync is not available in this target.');
}

const url = args && args.url;
if (!url || typeof url !== 'string') {
  throw __officeError('invalid_url', 'addin.openDialog requires a non-empty url.');
}

const opts = {};
if (args && typeof args.height === 'number') opts.height = args.height;
if (args && typeof args.width === 'number') opts.width = args.width;
if (args && typeof args.displayInIframe === 'boolean') opts.displayInIframe = args.displayInIframe;
if (args && typeof args.promptBeforeOpen === 'boolean') opts.promptBeforeOpen = args.promptBeforeOpen;

await new Promise((resolve, reject) => {
  try {
    Office.context.ui.displayDialogAsync(url, opts, (asyncResult) => {
      const status = asyncResult && asyncResult.status;
      if (status === Office.AsyncResultStatus.Failed || (asyncResult && asyncResult.error)) {
        const err = asyncResult.error || {};
        reject(__officeError(err.code ? 'dialog_' + err.code : 'dialog_open_failed', err.message || 'displayDialogAsync failed'));
        return;
      }
      // Persist the dialog handle on the global so subsequent close/message
      // tools can reach it without re-opening. Multiple opens replace the
      // handle — Office only allows one dialog at a time anyway.
      try {
        globalThis.__officeAddinMcpDialog = asyncResult.value;
      } catch (_) { /* ignore */ }
      resolve();
    });
  } catch (e) {
    reject(__officeError('dialog_open_threw', String(e && e.message || e)));
  }
});

return { result: { opened: true, url: url } };
