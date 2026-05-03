// @requires OneNoteApi 1.1
const data = await __runOneNote(async (context) => {
  const section = context.application.getActiveSection();
  const page = section.addPage(args.title || '');
  page.load('id,title');
  await context.sync();
  return { id: page.id, title: page.title };
});
return { result: data };
