// @requires WordApi 1.1
const data = await __runWord(async (context) => {
  const body = context.document.body;
  const location = args.location || 'End';
  const para = body.insertParagraph(args.text || '', location);
  para.load('text,style');
  await context.sync();
  return { text: para.text, style: para.style, location: location };
});
return { result: data };
