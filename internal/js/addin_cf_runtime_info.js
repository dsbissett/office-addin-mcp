// Probe the Custom Functions runtime for registered functions. Best-effort —
// the CF runtime's introspection surface is not part of the public Office.js
// API, so we read CustomFunctions._association.mappings if present and fall
// back to whatever metadata is reachable.
await __ensureOffice();

const out = {
  available: false,
  registered: [],
  mappings: null,
};

const cf = globalThis.CustomFunctions;
if (!cf) {
  return { result: out };
}
out.available = true;

try {
  if (cf._association && cf._association.mappings) {
    out.mappings = Object.assign({}, cf._association.mappings);
    out.registered = Object.keys(out.mappings);
  } else if (typeof cf.associations === 'object' && cf.associations) {
    out.mappings = Object.assign({}, cf.associations);
    out.registered = Object.keys(out.mappings);
  }
} catch (e) {
  out.error = String(e && e.message || e);
}

return { result: out };
