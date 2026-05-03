// @requires OneNoteApi 1.1
const data = await __runOneNote(async (context) => {
  const notebook = context.application.getActiveNotebook();
  notebook.load('id,name');
  notebook.sections.load('items/id,items/name');
  await context.sync();
  const items = notebook.sections.items.map((s) => ({ id: s.id, name: s.name }));
  return { notebookId: notebook.id, notebookName: notebook.name, sections: items, count: items.length };
});
return { result: data };
