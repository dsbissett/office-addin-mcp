// @requires PowerPointApi 1.1
const data = await __runPowerPoint(async (context) => {
  const pres = context.presentation;
  pres.load('title');
  pres.slides.load('items/id');
  await context.sync();
  return { title: pres.title || null, slideCount: pres.slides.items.length };
});
return { result: data };
