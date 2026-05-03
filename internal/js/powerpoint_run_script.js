// @requires PowerPointApi 1.1
//
// Runs an arbitrary user-supplied async script body inside PowerPoint.run.
// The body receives `context` (the RequestContext) and `args` (the
// user-supplied scriptArgs). It must `return` a JSON-serializable value.
const __script = args.script;
const __scriptArgs = args.scriptArgs || {};
if (typeof __script !== 'string' || __script.length === 0) {
  throw __officeError('invalid_script', 'script is required and must be a non-empty string.');
}
let __fn;
try {
  __fn = new Function(
    'context',
    'args',
    '"use strict";\nreturn (async () => {\n' + __script + '\n})();',
  );
} catch (e) {
  throw __officeError('script_compile_failed', String(e && e.message || e));
}
const data = await __runPowerPoint(async (context) => {
  return await __fn(context, __scriptArgs);
});
return { result: data };
