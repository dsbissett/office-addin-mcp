// @requires WordApi 1.1
const data = await __runWord(async (context) => {
  const selection = context.document.getSelection();
  selection.load('text');
  await context.sync();
  return { text: selection.text };
});
return { result: data };
