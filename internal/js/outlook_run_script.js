// @requires Mailbox 1.1
//
// Runs an arbitrary user-supplied async script body with Office.context.mailbox
// passed as `mailbox`. Outlook lacks a `<Host>.run` batched-context API; the
// preamble's __runOutlook helper just hands the mailbox object to the body.
const __script = args.script;
const __scriptArgs = args.scriptArgs || {};
if (typeof __script !== 'string' || __script.length === 0) {
  throw __officeError('invalid_script', 'script is required and must be a non-empty string.');
}
let __fn;
try {
  __fn = new Function(
    'mailbox',
    'args',
    '"use strict";\nreturn (async () => {\n' + __script + '\n})();',
  );
} catch (e) {
  throw __officeError('script_compile_failed', String(e && e.message || e));
}
const data = await __runOutlook(async (mailbox) => {
  return await __fn(mailbox, __scriptArgs);
});
return { result: data };
