// @requires ExcelApi 1.5
const data = await __runExcel(async (context) => {
  const parts = context.workbook.customXmlParts;
  parts.load('items/id,items/namespaceUri');
  await context.sync();
  return {
    parts: parts.items.map((p) => ({ id: p.id, namespaceUri: p.namespaceUri })),
  };
});
return { result: data };
