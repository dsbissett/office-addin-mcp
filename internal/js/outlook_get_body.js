// @requires Mailbox 1.1
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item || !item.body) {
    throw __officeError('outlook_no_item', 'No mailbox item with a body is currently selected.');
  }
  const coercionType = args.coercionType || (Office.CoercionType ? Office.CoercionType.Text : 'text');
  const value = await new Promise((resolve, reject) => {
    item.body.getAsync(coercionType, (r) => {
      if (r.status === 'succeeded') resolve(r.value);
      else reject(__officeError('outlook_get_body_failed', (r.error && r.error.message) || 'getAsync failed'));
    });
  });
  return { body: value, coercionType: coercionType };
});
return { result: data };
