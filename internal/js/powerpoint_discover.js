// Discovery payload: presentation-level snapshot + fingerprint for caching.
const data = await __runPowerPoint(async (context) => {
  const pres = context.presentation;
  pres.load(['title']);
  const slides = pres.slides;
  slides.load(['items/id', 'items/index']);
  await context.sync();
  const shapeHandles = [];
  for (let i = 0; i < slides.items.length; i++) {
    const sh = slides.items[i].shapes;
    sh.load(['items/id']);
    shapeHandles.push(sh);
  }
  await context.sync();
  let totalShapes = 0;
  for (const h of shapeHandles) totalShapes += h.items.length;
  const fingerprint = 'pp:s' + slides.items.length + ':sh' + totalShapes;
  return {
    filePath: pres.title || '',
    fingerprint: fingerprint,
    title: pres.title || null,
    slideCount: slides.items.length,
    shapeCount: totalShapes,
    slides: slides.items.map((s, i) => ({ id: s.id, index: s.index, shapeCount: shapeHandles[i].items.length })),
  };
});
return { result: data };
