// @requires WordApi 1.1
const data = await __runWord(async (context) => {
  const body = context.document.body;
  const location = args.location || 'Replace';
  body.insertText(args.text || '', location);
  await context.sync();
  return { ok: true, location: location };
});
return { result: data };
