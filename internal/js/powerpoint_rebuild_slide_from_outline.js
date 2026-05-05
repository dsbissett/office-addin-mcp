// @requires PowerPointApi 1.4
//
// Workflow tool: rewrite the title and body bullets of an existing slide in a
// single PowerPoint.run. Identifies title vs body shape by inspecting shape
// names (PowerPoint conventions: "Title", "Title 1", … vs "Content Placeholder",
// "Body", "Subtitle"). Falls back to the first text-bearing shape when no
// title-like shape is found.
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
  slide.shapes.load('items/name,items/type');
  await context.sync();

  const shapes = slide.shapes.items;
  let titleShape = null;
  let bodyShape = null;
  for (let i = 0; i < shapes.length; i++) {
    const name = (shapes[i].name || '').toLowerCase();
    if (!titleShape && (name.indexOf('title') === 0 || name === 'subtitle' || name.indexOf('subtitle ') === 0)) {
      titleShape = shapes[i];
    } else if (!bodyShape && (name.indexOf('content') !== -1 || name.indexOf('body') !== -1 || name.indexOf('placeholder') !== -1)) {
      bodyShape = shapes[i];
    }
  }
  if (!titleShape && shapes.length > 0) titleShape = shapes[0];
  if (!bodyShape && shapes.length > 1) bodyShape = shapes[1];

  let titleSet = false;
  if (typeof args.title === 'string' && titleShape) {
    if (titleShape.textFrame && titleShape.textFrame.textRange) {
      titleShape.textFrame.textRange.text = args.title;
      titleSet = true;
    }
  }

  let bulletsSet = 0;
  const bullets = Array.isArray(args.bullets) ? args.bullets : null;
  if (bullets && bodyShape && bodyShape.textFrame && bodyShape.textFrame.textRange) {
    bodyShape.textFrame.textRange.text = bullets.join('\n');
    bulletsSet = bullets.length;
  }

  await context.sync();
  return {
    slideIndex: idx,
    slideId: slide.id,
    titleSet: titleSet,
    bulletsSet: bulletsSet,
    titleShapeName: titleShape ? titleShape.name : null,
    bodyShapeName: bodyShape ? bodyShape.name : null,
  };
});
return { result: data };
