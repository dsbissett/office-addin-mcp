// @requires ExcelApi 1.9
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  const found = range.findAllOrNullObject(args.text, {
    completeMatch: !!args.completeMatch,
    matchCase: !!args.matchCase,
  });
  found.load(['address', 'cellCount', 'isNullObject']);
  await context.sync();
  if (found.isNullObject) {
    return { found: false };
  }
  return {
    found: true,
    address: found.address,
    cellCount: found.cellCount,
  };
});
return { result: data };
