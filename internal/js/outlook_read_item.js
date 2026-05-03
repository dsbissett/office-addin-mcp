// @requires Mailbox 1.1
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox.item;
  if (!item) {
    throw __officeError('outlook_no_item', 'No mailbox item is currently selected.');
  }
  return {
    subject: item.subject || null,
    itemType: item.itemType || null,
    itemClass: item.itemClass || null,
    conversationId: item.conversationId || null,
    dateTimeCreated: item.dateTimeCreated ? item.dateTimeCreated.toISOString() : null,
    dateTimeModified: item.dateTimeModified ? item.dateTimeModified.toISOString() : null,
    itemId: item.itemId || null,
  };
});
return { result: data };
