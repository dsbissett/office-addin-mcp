// @requires WordApi 1.1
const data = await __runWord(async (context) => {
  const paragraphs = context.document.body.paragraphs;
  paragraphs.load('items/text,items/style');
  await context.sync();
  const items = paragraphs.items.map((p) => ({ text: p.text, style: p.style }));
  return { paragraphs: items, count: items.length };
});
return { result: data };
