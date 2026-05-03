// @requires OneNoteApi 1.1
const data = await __runOneNote(async (context) => {
  const notebooks = context.application.notebooks;
  notebooks.load('items/id,items/name');
  await context.sync();
  const items = notebooks.items.map((n) => ({ id: n.id, name: n.name }));
  return { notebooks: items, count: items.length };
});
return { result: data };
