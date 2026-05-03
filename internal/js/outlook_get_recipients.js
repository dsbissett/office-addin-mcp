// @requires Mailbox 1.1
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item) {
    throw __officeError('outlook_no_item', 'No mailbox item is currently selected.');
  }

  function readField(field) {
    if (!field) return Promise.resolve([]);
    // Compose mode: Recipients with getAsync. Read mode: array of EmailAddressDetails.
    if (typeof field.getAsync === 'function') {
      return new Promise((resolve, reject) => {
        field.getAsync((r) => {
          if (r.status === 'succeeded') resolve(r.value || []);
          else reject(__officeError('outlook_get_recipients_failed', (r.error && r.error.message) || 'getAsync failed'));
        });
      });
    }
    return Promise.resolve(Array.isArray(field) ? field : []);
  }

  const [to, cc] = await Promise.all([readField(item.to), readField(item.cc)]);
  const map = (rs) => rs.map((r) => ({
    displayName: r.displayName || null,
    emailAddress: r.emailAddress || null,
    recipientType: r.recipientType || null,
  }));
  return { to: map(to), cc: map(cc) };
});
return { result: data };
