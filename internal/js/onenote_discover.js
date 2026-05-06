// Discovery payload: notebook + section snapshot with fingerprint.
const data = await __runOneNote(async (context) => {
  const app = context.application;
  const notebooks = app.notebooks;
  notebooks.load(['items/id', 'items/name']);
  const activeSection = app.getActiveSection();
  activeSection.load(['id', 'name']);
  activeSection.pages.load(['items/id', 'items/title']);
  await context.sync();

  const fingerprint =
    'on:nb' + notebooks.items.length +
    ':p' + activeSection.pages.items.length;
  return {
    filePath: activeSection.name || '',
    fingerprint: fingerprint,
    notebooks: notebooks.items.map((n) => ({ id: n.id, name: n.name })),
    activeSectionId: activeSection.id,
    activeSectionName: activeSection.name,
    pages: activeSection.pages.items.map((p) => ({ id: p.id, title: p.title })),
    pageCount: activeSection.pages.items.length,
  };
});
return { result: data };
