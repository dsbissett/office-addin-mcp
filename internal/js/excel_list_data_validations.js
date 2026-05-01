// @requires ExcelApi 1.8
const data = await __runExcel(async (context) => {
  const range = __resolveRange(context, args.address, args.sheet);
  range.load('address');
  const dv = range.dataValidation;
  dv.load(['type', 'rule', 'errorAlert', 'prompt', 'ignoreBlanks', 'valid']);
  await context.sync();
  return {
    address: range.address,
    dataValidation: {
      type: dv.type,
      rule: dv.rule,
      errorAlert: dv.errorAlert,
      prompt: dv.prompt,
      ignoreBlanks: dv.ignoreBlanks,
      valid: dv.valid,
    },
  };
});
return { result: data };
