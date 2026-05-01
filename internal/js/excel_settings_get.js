// Read add-in document settings from Office.context.document.settings.
// Does not call Excel.run — settings live on the Office surface.
await __ensureOffice();

const settings =
  (Office && Office.context && Office.context.document && Office.context.document.settings) ||
  null;
if (!settings) {
  throw __officeError('settings_unavailable', 'Office.context.document.settings not available');
}

if (args && args.key) {
  return { result: { key: args.key, value: settings.get(args.key) } };
}
if (typeof settings.getAll === 'function') {
  return { result: { settings: settings.getAll() } };
}
throw __officeError('settings_get_all_unsupported', 'settings.getAll is not available on this host');
