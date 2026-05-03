// @requires WordApi 1.3
const data = await __runWord(async (context) => {
  const props = context.document.properties;
  props.load('title,subject,author,company,keywords,comments,category,manager,lastAuthor,creationDate,lastSaveTime,revisionNumber,template,format');
  await context.sync();
  return {
    title: props.title,
    subject: props.subject,
    author: props.author,
    company: props.company,
    keywords: props.keywords,
    comments: props.comments,
    category: props.category,
    manager: props.manager,
    lastAuthor: props.lastAuthor,
    creationDate: props.creationDate,
    lastSaveTime: props.lastSaveTime,
    revisionNumber: props.revisionNumber,
    template: props.template,
    format: props.format,
  };
});
return { result: data };
