// Discovery payload: document-level snapshot + fingerprint for caching.
const data = await __runWord(async (context) => {
  const doc = context.document;
  const body = doc.body;
  const props = doc.properties;
  body.load(['text']);
  doc.load(['saved']);
  props.load(['title', 'author', 'lastAuthor', 'creationDate', 'lastSaveTime']);
  const sections = doc.sections;
  sections.load(['items']);
  const ct = doc.contentControls;
  ct.load(['items/id', 'items/title', 'items/tag', 'items/type']);
  await context.sync();

  const text = body.text || '';
  const wordCount = text.split(/\s+/).filter(Boolean).length;
  const fingerprint =
    'doc:s' + sections.items.length +
    ':cc' + ct.items.length +
    ':w' + wordCount;
  return {
    filePath: props.title || '',
    fingerprint: fingerprint,
    title: props.title || null,
    author: props.author || null,
    lastAuthor: props.lastAuthor || null,
    sectionCount: sections.items.length,
    contentControls: ct.items.map((c) => ({ id: c.id, title: c.title, tag: c.tag, type: c.type })),
    wordCount: wordCount,
    saved: doc.saved,
  };
});
return { result: data };
