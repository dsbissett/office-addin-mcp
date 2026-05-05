// @requires WordApi 1.1
//
// Workflow tool: apply a batch of {find, replace, matchCase?, matchWholeWord?}
// edits to the document body in a single Word.run. One CDP round-trip per
// call, regardless of edit count.
const data = await __runWord(async (context) => {
  const edits = Array.isArray(args.edits) ? args.edits : [];
  if (edits.length === 0) {
    throw __officeError('no_edits', 'edits must contain at least one find/replace pair.');
  }
  const body = context.document.body;
  const matchSets = [];
  for (let i = 0; i < edits.length; i++) {
    const e = edits[i];
    if (!e || typeof e.find !== 'string' || e.find.length === 0) {
      throw __officeError('invalid_edit', 'edits[' + i + '].find is required and must be non-empty.');
    }
    const opts = {};
    if (typeof e.matchCase === 'boolean') opts.matchCase = e.matchCase;
    if (typeof e.matchWholeWord === 'boolean') opts.matchWholeWord = e.matchWholeWord;
    const found = body.search(e.find, opts);
    found.load('items');
    matchSets.push({ idx: i, edit: e, results: found });
  }
  await context.sync();

  const summary = [];
  for (let m = 0; m < matchSets.length; m++) {
    const set = matchSets[m];
    const replacement = typeof set.edit.replace === 'string' ? set.edit.replace : '';
    const items = set.results.items;
    for (let i = 0; i < items.length; i++) {
      items[i].insertText(replacement, 'Replace');
    }
    summary.push({
      find: set.edit.find,
      replace: replacement,
      replaced: items.length,
    });
  }
  await context.sync();

  return { edits: summary };
});
return { result: data };
