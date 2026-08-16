// Dawa24 Frontend Application Script
// HTMX + CSRF Token Header Integration

document.addEventListener('DOMContentLoaded', () => {
  // Read dawa_csrf cookie and attach X-CSRF-Token to all HTMX requests
  document.body.addEventListener('htmx:configRequest', (evt) => {
    const csrfToken = getCookie('dawa_csrf');
    if (csrfToken) {
      evt.detail.headers['X-CSRF-Token'] = csrfToken;
    }
  });

  // Handle HTMX response errors gracefully
  document.body.addEventListener('htmx:responseError', (evt) => {
    console.error('Request failed:', evt.detail.xhr.status, evt.detail.xhr.responseText);
  });
});

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}
