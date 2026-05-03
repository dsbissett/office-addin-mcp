// @requires Mailbox 1.1
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item || !item.body || typeof item.body.setAsync !== 'function') {
    throw __officeError('outlook_set_body_unavailable', 'Body cannot be set on this item (not in compose mode?).');
  }
  const coercionType = args.coercionType || (Office.CoercionType ? Office.CoercionType.Text : 'text');
  await new Promise((resolve, reject) => {
    item.body.setAsync(args.content || '', { coercionType: coercionType }, (r) => {
      if (r.status === 'succeeded') resolve();
      else reject(__officeError('outlook_set_body_failed', (r.error && r.error.message) || 'setAsync failed'));
    });
  });
  return { ok: true, coercionType: coercionType };
});
return { result: data };
