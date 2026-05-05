// @requires OneNoteApi 1.1
//
// Workflow tool: append HTML or an outline (list of strings) to the active
// (or named) OneNote page in one call. Replaces the manual addOutline +
// appendHtml dance.
const data = await __runOneNote(async (context) => {
  let page;
  if (typeof args.pageId === 'string' && args.pageId) {
    page = context.application.getPageById ? context.application.getPageById(args.pageId) : null;
    if (!page) {
      throw __officeError('onenote_get_page_unavailable', 'getPageById is not available in this OneNote version; omit pageId to append to the active page.');
    }
  } else {
    page = context.application.getActivePage();
  }
  page.load('id,title');

  let appended = 0;
  if (typeof args.html === 'string' && args.html.length > 0) {
    page.addOutline(40, 40, args.html);
    appended++;
  }
  if (Array.isArray(args.bullets) && args.bullets.length > 0) {
    const html = '<ul>' + args.bullets.map((b) => '<li>' + __escapeHtml(String(b)) + '</li>').join('') + '</ul>';
    page.addOutline(40, 40, html);
    appended++;
  }
  if (appended === 0) {
    throw __officeError('nothing_to_append', 'appendToPage requires html or bullets.');
  }

  await context.sync();
  return { id: page.id, title: page.title, outlinesAppended: appended };
});
return { result: data };

function __escapeHtml(s) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
