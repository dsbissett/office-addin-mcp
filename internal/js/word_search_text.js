// @requires WordApi 1.1
const data = await __runWord(async (context) => {
  const body = context.document.body;
  const opts = {};
  if (typeof args.matchCase === 'boolean') opts.matchCase = args.matchCase;
  if (typeof args.matchWholeWord === 'boolean') opts.matchWholeWord = args.matchWholeWord;
  const results = body.search(args.query || '', opts);
  results.load('items/text');
  await context.sync();
  const items = results.items.map((r) => ({ text: r.text }));
  return { matches: items, count: items.length };
});
return { result: data };
