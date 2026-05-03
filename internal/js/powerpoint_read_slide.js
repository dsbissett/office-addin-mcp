// @requires PowerPointApi 1.2
const data = await __runPowerPoint(async (context) => {
  const slides = context.presentation.slides;
  slides.load('items/id');
  await context.sync();
  const idx = typeof args.slideIndex === 'number' ? args.slideIndex : 0;
  if (idx < 0 || idx >= slides.items.length) {
    throw __officeError(
      'powerpoint_slide_out_of_range',
      'slideIndex ' + idx + ' is out of range (presentation has ' + slides.items.length + ' slide(s)).',
    );
  }
  const slide = slides.items[idx];
  slide.shapes.load('items/name,items/type,items/left,items/top,items/width,items/height');
  await context.sync();
  const shapes = slide.shapes.items.map((s) => ({
    name: s.name,
    type: s.type,
    left: s.left,
    top: s.top,
    width: s.width,
    height: s.height,
  }));
  return { slideIndex: idx, slideId: slide.id, shapes: shapes };
});
return { result: data };
