// @requires Mailbox 1.1
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item || !item.subject || typeof item.subject.setAsync !== 'function') {
    throw __officeError('outlook_set_subject_unavailable', 'Subject cannot be set on this item (not in compose mode?).');
  }
  await new Promise((resolve, reject) => {
    item.subject.setAsync(args.subject || '', (r) => {
      if (r.status === 'succeeded') resolve();
      else reject(__officeError('outlook_set_subject_failed', (r.error && r.error.message) || 'setAsync failed'));
    });
  });
  return { ok: true };
});
return { result: data };
