// @requires Mailbox 1.1
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item) {
    throw __officeError('outlook_no_item', 'No mailbox item is currently selected.');
  }
  // Compose mode: subject is a Subject object with getAsync. Read mode:
  // subject is a plain string property.
  if (item.subject && typeof item.subject.getAsync === 'function') {
    const value = await new Promise((resolve, reject) => {
      item.subject.getAsync((r) => {
        if (r.status === 'succeeded') resolve(r.value);
        else reject(__officeError('outlook_get_subject_failed', (r.error && r.error.message) || 'getAsync failed'));
      });
    });
    return { subject: value, mode: 'compose' };
  }
  return { subject: typeof item.subject === 'string' ? item.subject : null, mode: 'read' };
});
return { result: data };
