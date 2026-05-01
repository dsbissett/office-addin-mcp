// Close the currently open Office dialog. Looks up the handle stashed by
// addin.openDialog; if none is found, returns closed=false.
await __ensureOffice();

const handle = globalThis.__officeAddinMcpDialog;
if (!handle || typeof handle.close !== 'function') {
  return { result: { closed: false } };
}
try {
  handle.close();
} catch (e) {
  throw __officeError('dialog_close_failed', String(e && e.message || e));
}
try { delete globalThis.__officeAddinMcpDialog; } catch (_) { /* ignore */ }
return { result: { closed: true } };
