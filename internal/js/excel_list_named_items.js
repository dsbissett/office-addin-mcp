// @requires ExcelApi 1.4
const data = await __runExcel(async (context) => {
  const names = context.workbook.names;
  names.load('items/name,items/type,items/value,items/visible,items/comment,items/formula');
  await context.sync();
  return {
    names: names.items.map((n) => ({
      name: n.name,
      type: n.type,
      value: n.value,
      formula: n.formula,
      visible: n.visible,
      comment: n.comment,
    })),
  };
});
return { result: data };
