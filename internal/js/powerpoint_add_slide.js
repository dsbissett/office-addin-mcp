// @requires PowerPointApi 1.3
const data = await __runPowerPoint(async (context) => {
  const slide = context.presentation.slides.add();
  slide.load('id');
  await context.sync();
  return { id: slide.id };
});
return { result: data };
