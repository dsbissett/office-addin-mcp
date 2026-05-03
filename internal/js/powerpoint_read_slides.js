// @requires PowerPointApi 1.2
const data = await __runPowerPoint(async (context) => {
  const slides = context.presentation.slides;
  slides.load('items/id,items/shapes/items/name');
  await context.sync();
  const items = slides.items.map((s, idx) => ({
    index: idx,
    id: s.id,
    shapeNames: s.shapes.items.map((shape) => shape.name),
  }));
  return { slides: items, count: items.length };
});
return { result: data };
