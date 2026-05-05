// Workflow tool: enumerate slides + shapes, project into records, run
// __queryEngine. Useful for "find slides with chart shapes" or "count
// shapes per slide".
const data = await __runPowerPoint(async (context) => {
  const slides = context.presentation.slides;
  slides.load(['items/id', 'items/index']);
  await context.sync();
  const shapeHandles = [];
  for (let i = 0; i < slides.items.length; i++) {
    const sh = slides.items[i].shapes;
    sh.load(['items/id', 'items/name', 'items/type', 'items/left', 'items/top', 'items/width', 'items/height']);
    shapeHandles.push({ slide: slides.items[i], shapes: sh });
  }
  await context.sync();
  const records = [];
  for (const h of shapeHandles) {
    for (const s of h.shapes.items) {
      records.push({
        slideId: h.slide.id,
        slideIndex: h.slide.index,
        shapeId: s.id,
        name: s.name,
        type: s.type,
        left: s.left,
        top: s.top,
        width: s.width,
        height: s.height,
      });
    }
  }
  const result = __queryEngine(records, args.query || {});
  return {
    slideCount: slides.items.length,
    shapeCount: records.length,
    rows: result.rows,
    count: result.count,
    limited: result.truncated,
  };
});
return { result: data };
