// Discovery payload: snapshot the active mail item + a coarse fingerprint.
const data = await __runOutlook(async (mailbox) => {
  const item = mailbox && mailbox.item ? mailbox.item : null;
  const userProfile = mailbox && mailbox.userProfile ? mailbox.userProfile : null;
  const summary = {
    userEmail: userProfile && userProfile.emailAddress ? userProfile.emailAddress : null,
    userName: userProfile && userProfile.displayName ? userProfile.displayName : null,
    hostMode: item && item.itemType ? item.itemType : null,
    activeItemId: item && item.itemId ? item.itemId : null,
    conversationId: item && item.conversationId ? item.conversationId : null,
  };
  const fingerprint =
    'outlook:u' + (summary.userEmail || '') +
    ':i' + (summary.activeItemId || 'none');
  return Object.assign({ filePath: summary.userEmail || '', fingerprint: fingerprint }, summary);
});
return { result: data };
