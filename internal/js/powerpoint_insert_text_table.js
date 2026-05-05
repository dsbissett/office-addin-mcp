// @requires PowerPointApi 1.5
//
// Workflow building block: insert a tab/newline-formatted text table onto an
// existing slide as a single text box. Used by office.embed to land Excel
// values on a PowerPoint slide; PowerPoint's Office.js surface lacks a true
// addTable as of API 1.5, so this is the most reliable shape-based fallback.
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
  const rows = Array.isArray(args.rows) ? args.rows : [];
  if (rows.length === 0) {
    throw __officeError('no_rows', 'rows must be a non-empty 2-D array.');
  }
  const text = rows
    .map((row) => (Array.isArray(row) ? row.map((c) => (c == null ? '' : String(c))).join('\t') : String(row)))
    .join('\n');
  const left = typeof args.left === 'number' ? args.left : 50;
  const top = typeof args.top === 'number' ? args.top : 50;
  const width = typeof args.width === 'number' ? args.width : 600;
  const height = typeof args.height === 'number' ? args.height : 400;
  const opts = { left: left, top: top, width: width, height: height };
  const shape = slide.shapes.addTextBox(text, opts);
  shape.load('id,name');
  await context.sync();
  return {
    slideIndex: idx,
    slideId: slide.id,
    shapeId: shape.id,
    shapeName: shape.name,
    rowCount: rows.length,
    columnCount: Array.isArray(rows[0]) ? rows[0].length : 1,
  };
});
return { result: data };
