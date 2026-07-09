// Display helpers shared by the rail and editor apps — the client-side twins of
// the templ methodClass/statusClass/preview funcs, so app.css styles unchanged.
export const methodClass = (m) => 'method method-' + (m || '').toUpperCase().trim()
export const statusClass = (s) => 'status status-' + Math.floor((parseInt(s, 10) || 0) / 100) + 'xx'
export const preview = (s) => {
  s = (s || '').replace(/\s+/g, ' ').trim()
  return s.length > 60 ? s.slice(0, 60) + '…' : s
}
export const dimLabel = (dim) => {
  if (dim.startsWith('header:')) return 'header ' + dim.slice(7)
  if (dim.startsWith('query:')) return 'query ' + dim.slice(6)
  return dim
}
export const gotDisplay = (got) => (got === '' ? '(absent)' : got)
