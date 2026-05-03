// @requires WordApi 1.1
const data = await __runWord(async (context) => {
  const body = context.document.body;
  body.load('text');
  await context.sync();
  return { text: body.text };
});
return { result: data };
