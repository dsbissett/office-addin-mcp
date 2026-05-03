// @requires OneNoteApi 1.1
const data = await __runOneNote(async (context) => {
  const section = context.application.getActiveSection();
  section.load('id,name');
  section.pages.load('items/id,items/title');
  await context.sync();
  const items = section.pages.items.map((p) => ({ id: p.id, title: p.title }));
  return { sectionId: section.id, sectionName: section.name, pages: items, count: items.length };
});
return { result: data };
