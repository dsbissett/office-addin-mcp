// Workflow tool: enumerate items in a mailbox folder via REST/EWS-fallback,
// project into records, then run __queryEngine. Outlook does not expose
// arbitrary folder enumeration through Office.js APIs, so this lists items
// reachable from the active item context (current folder via custom
// properties + restApi when available).
const data = await __runOutlook(async (mailbox) => {
  const items = [];
  if (mailbox && mailbox.item) {
    const it = mailbox.item;
    // Best-effort projection of the active item; folder-wide enumeration
    // requires a REST token + URL the host did not expose synchronously.
    items.push({
      itemId: it.itemId || null,
      subject: it.subject || (typeof it.subject === 'object' && it.subject.getAsync ? null : null),
      conversationId: it.conversationId || null,
      itemType: it.itemType || null,
      from: it.from && it.from.emailAddress ? it.from.emailAddress : null,
      to: Array.isArray(it.to) ? it.to.map((r) => r && r.emailAddress).filter(Boolean) : [],
      dateTimeCreated: it.dateTimeCreated ? it.dateTimeCreated.toISOString() : null,
    });
  }
  const result = __queryEngine(items, args.query || {});
  return {
    folder: 'currentItem',
    rows: result.rows,
    count: result.count,
    limited: result.truncated,
    note: 'Outlook query reads the active item context. Folder-wide enumeration requires REST token; out of scope for v1.',
  };
});
return { result: data };
