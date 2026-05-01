// @requires ExcelApi 1.10
const data = await __runExcel(async (context) => {
  const ws = args.sheet
    ? context.workbook.worksheets.getItem(args.sheet)
    : context.workbook.worksheets.getActiveWorksheet();
  ws.load('name');
  const comments = ws.comments;
  comments.load(
    'items/id,items/authorName,items/authorEmail,items/content,items/creationDate,items/resolved',
  );
  await context.sync();
  const cellRanges = comments.items.map((c) => c.getLocation().load('address'));
  const replyLists = comments.items.map((c) => {
    const r = c.replies;
    r.load('items/id,items/authorName,items/content,items/creationDate');
    return r;
  });
  await context.sync();
  return {
    worksheet: ws.name,
    comments: comments.items.map((c, i) => ({
      id: c.id,
      author: c.authorName,
      authorEmail: c.authorEmail,
      content: c.content,
      creationDate: c.creationDate,
      resolved: c.resolved,
      address: cellRanges[i].address,
      replies: replyLists[i].items.map((r) => ({
        id: r.id,
        author: r.authorName,
        content: r.content,
        creationDate: r.creationDate,
      })),
    })),
  };
});
return { result: data };
