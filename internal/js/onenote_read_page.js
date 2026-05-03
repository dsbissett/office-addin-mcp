// @requires OneNoteApi 1.1
const data = await __runOneNote(async (context) => {
  const page = context.application.getActivePage();
  page.load('id,title');
  page.contents.load('items/id,items/type');
  await context.sync();
  const contents = page.contents.items.map((c) => ({ id: c.id, type: c.type }));
  return { id: page.id, title: page.title, contents: contents };
});
return { result: data };
