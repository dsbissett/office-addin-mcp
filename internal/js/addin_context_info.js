// Probe Office.context for the loaded add-in's identity, host, and supported
// requirement sets. Does not call Excel.run — surface introspection only.
await __ensureOffice();

const sets = (args && Array.isArray(args.requirementSets)) ? args.requirementSets : [];
const support = {};
for (const entry of sets) {
  const name = (entry && entry.name) || '';
  const minVersion = (entry && entry.minVersion) || '1.1';
  if (!name) continue;
  let supported = false;
  try {
    supported = !!Office.context.requirements.isSetSupported(name, minVersion);
  } catch (_) {
    supported = false;
  }
  support[name + '@' + minVersion] = supported;
}

const ctx = (Office && Office.context) || {};
const host = ctx.host || (Office.HostType && Office.HostType.Excel) || null;
const platform = ctx.platform || null;
const docUrl = (ctx.document && ctx.document.url) || null;
const contentLanguage = ctx.contentLanguage || null;
const displayLanguage = ctx.displayLanguage || null;
const officeTheme = (ctx.officeTheme && {
  bodyBackgroundColor: ctx.officeTheme.bodyBackgroundColor,
  bodyForegroundColor: ctx.officeTheme.bodyForegroundColor,
}) || null;

return {
  result: {
    host: host,
    platform: platform,
    contentLanguage: contentLanguage,
    displayLanguage: displayLanguage,
    documentUrl: docUrl,
    theme: officeTheme,
    requirementSets: support,
  },
};
