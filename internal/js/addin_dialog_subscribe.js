// Drain queued dialog messages. The first call installs a DialogMessageReceived
// handler on the active dialog handle; subsequent calls return the messages
// queued since the previous call and clear the buffer. The buffer lives on
// globalThis so it survives between Office.js script invocations within the
// same target session.
await __ensureOffice();

const handle = globalThis.__officeAddinMcpDialog;
if (!handle) {
  return { result: { installed: false, messages: [] } };
}

if (!globalThis.__officeAddinMcpDialogQueue) {
  globalThis.__officeAddinMcpDialogQueue = [];
}
if (!globalThis.__officeAddinMcpDialogSubscribed) {
  try {
    handle.addEventHandler(Office.EventType.DialogMessageReceived, (arg) => {
      try {
        globalThis.__officeAddinMcpDialogQueue.push({
          type: 'message',
          origin: (arg && arg.origin) || null,
          message: (arg && arg.message) || '',
          at: Date.now(),
        });
      } catch (_) { /* ignore */ }
    });
    handle.addEventHandler(Office.EventType.DialogEventReceived, (arg) => {
      try {
        globalThis.__officeAddinMcpDialogQueue.push({
          type: 'event',
          code: (arg && arg.error) || 0,
          at: Date.now(),
        });
      } catch (_) { /* ignore */ }
    });
    globalThis.__officeAddinMcpDialogSubscribed = true;
  } catch (e) {
    throw __officeError('dialog_subscribe_failed', String(e && e.message || e));
  }
}

const drained = globalThis.__officeAddinMcpDialogQueue;
globalThis.__officeAddinMcpDialogQueue = [];
return { result: { installed: true, messages: drained } };
