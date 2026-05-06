// @requires Mailbox 1.1
//
// Workflow tool: in compose mode, set both subject and body in one call.
// Used by agents drafting a reply where the prior tool surface forced two
// round-trips (set_subject + set_body).
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item) {
    throw __officeError('outlook_no_item', 'No mailbox item is currently selected.');
  }
  if (!item.body || typeof item.body.setAsync !== 'function') {
    throw __officeError('outlook_compose_required', 'draftReply requires a compose-mode item (body.setAsync unavailable).');
  }
  const coercionType = args.coercionType || (Office.CoercionType ? Office.CoercionType.Html : 'html');
  const promises = [];

  if (typeof args.subject === 'string' && item.subject && typeof item.subject.setAsync === 'function') {
    promises.push(new Promise((resolve, reject) => {
      item.subject.setAsync(args.subject, (r) => {
        if (r.status === 'succeeded') resolve();
        else reject(__officeError('outlook_set_subject_failed', (r.error && r.error.message) || 'subject.setAsync failed'));
      });
    }));
  }

  if (typeof args.body === 'string') {
    promises.push(new Promise((resolve, reject) => {
      item.body.setAsync(args.body, { coercionType: coercionType }, (r) => {
        if (r.status === 'succeeded') resolve();
        else reject(__officeError('outlook_set_body_failed', (r.error && r.error.message) || 'body.setAsync failed'));
      });
    }));
  }

  if (promises.length === 0) {
    throw __officeError('nothing_to_set', 'draftReply requires at least one of: subject, body.');
  }

  await Promise.all(promises);
  return {
    ok: true,
    subjectSet: typeof args.subject === 'string',
    bodySet: typeof args.body === 'string',
    coercionType: coercionType,
  };
});
return { result: data };
