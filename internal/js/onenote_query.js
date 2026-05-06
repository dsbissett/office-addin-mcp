// Workflow tool: enumerate pages in the active section, project into
// records (id, title), then run __queryEngine. Returns small filtered
// answers instead of the full page list.
const data = await __runOneNote(async (context) => {
  const section = context.application.getActiveSection();
  section.load(['id', 'name']);
  section.pages.load(['items/id', 'items/title']);
  await context.sync();
  const records = section.pages.items.map((p) => ({ id: p.id, title: p.title }));
  const result = __queryEngine(records, args.query || {});
  return {
    sectionId: section.id,
    sectionName: section.name,
    pageCount: records.length,
    rows: result.rows,
    count: result.count,
    limited: result.truncated,
  };
});
return { result: data };
