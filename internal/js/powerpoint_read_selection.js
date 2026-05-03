// @requires PowerPointApi 1.2
const data = await __runPowerPoint(async (context) => {
  const sel = context.presentation.getSelectedSlides();
  sel.load('items/id');
  await context.sync();
  const items = sel.items.map((s) => ({ id: s.id }));
  return { slides: items, count: items.length };
});
return { result: data };
